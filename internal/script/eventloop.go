package script

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/dop251/goja"
)

// MaxEventLoopDuration bounds how long a script's pending timers and
// pm.sendRequest callbacks may keep it alive. Polling scripts (setTimeout
// re-checking a read model) legitimately run well past DefaultScriptTimeout,
// but an endless poll must not hang a request forever.
const MaxEventLoopDuration = 2 * time.Minute

var (
	ErrScriptTimeout    = errors.New("script execution timeout")
	ErrEventLoopTimeout = errors.New("pending timers and callbacks did not finish")
)

type scheduledJob struct {
	id   int64
	due  time.Time
	fn   goja.Callable
	args []goja.Value
}

// EventLoop runs a script and then the callbacks it scheduled (setTimeout,
// pm.sendRequest), one at a time on the VM's goroutine, as Postman does.
// Every synchronous JS segment gets its own timeout, so time spent waiting on
// timers or HTTP never counts against a script's CPU budget.
type EventLoop struct {
	vm      *goja.Runtime
	timeout time.Duration
	nextID  int64
	pending []scheduledJob
	segment *time.Timer
}

func newEventLoop(vm *goja.Runtime, timeout time.Duration) *EventLoop {
	return &EventLoop{vm: vm, timeout: timeout}
}

func (l *EventLoop) install() error {
	if err := l.vm.Set("setTimeout", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(l.vm.NewTypeError("setTimeout requires a function"))
		}
		delay := time.Duration(call.Argument(1).ToInteger()) * time.Millisecond
		var args []goja.Value
		if len(call.Arguments) > 2 {
			args = call.Arguments[2:]
		}
		return l.vm.ToValue(l.schedule(delay, fn, args...))
	}); err != nil {
		return fmt.Errorf("failed to set setTimeout: %w", err)
	}

	if err := l.vm.Set("clearTimeout", func(call goja.FunctionCall) goja.Value {
		l.cancel(call.Argument(0).ToInteger())
		return goja.Undefined()
	}); err != nil {
		return fmt.Errorf("failed to set clearTimeout: %w", err)
	}

	return nil
}

func (l *EventLoop) schedule(delay time.Duration, fn goja.Callable, args ...goja.Value) int64 {
	l.nextID++
	l.pending = append(l.pending, scheduledJob{id: l.nextID, due: time.Now().Add(delay), fn: fn, args: args})
	return l.nextID
}

func (l *EventLoop) cancel(id int64) {
	for i, job := range l.pending {
		if job.id == id {
			l.pending = slices.Delete(l.pending, i, i+1)
			return
		}
	}
}

// RunScript runs code under the per-segment timeout.
func (l *EventLoop) RunScript(code string) error {
	return l.guard(func() error {
		_, err := l.vm.RunString(code)
		return err
	})
}

// Drain runs scheduled callbacks in due order (ties in scheduling order)
// until none remain or MaxEventLoopDuration passes. A callback that throws
// is reported but does not stop the remaining ones, matching Postman.
func (l *EventLoop) Drain() []error {
	var errs []error
	deadline := time.Now().Add(MaxEventLoopDuration)

	for len(l.pending) > 0 {
		sort.SliceStable(l.pending, func(i, j int) bool { return l.pending[i].due.Before(l.pending[j].due) })
		job := l.pending[0]
		if job.due.After(deadline) || time.Now().After(deadline) {
			l.pending = nil
			return append(errs, fmt.Errorf("%w within %v", ErrEventLoopTimeout, MaxEventLoopDuration))
		}
		l.pending = l.pending[1:]

		time.Sleep(time.Until(job.due))
		if err := l.guard(func() error {
			_, err := job.fn(goja.Undefined(), job.args...)
			return err
		}); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// waitOutsideSegment runs a blocking host call (e.g. HTTP) with the segment
// timer paused, so a slow server is not mistaken for a runaway script. The
// timer restarts afresh afterwards; the host call bounds its own wait.
func (l *EventLoop) waitOutsideSegment(wait func()) {
	if l.segment == nil {
		wait()
		return
	}
	l.segment.Stop()
	wait()
	l.segment.Reset(l.timeout)
}

func (l *EventLoop) guard(run func() error) error {
	l.segment = time.AfterFunc(l.timeout, func() { l.vm.Interrupt(ErrScriptTimeout) })
	err := run()
	l.segment.Stop()
	l.segment = nil
	l.vm.ClearInterrupt()

	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		return fmt.Errorf("%w after %v", ErrScriptTimeout, l.timeout)
	}
	return err
}
