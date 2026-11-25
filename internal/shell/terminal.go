package shell

import (
	"context"
	"io"
	"os/exec"
	"sync"
)

// ShowOutput tees cmd's output to stdout and stderr as well as to the
// writers already set on it.
func ShowOutput(cmd *exec.Cmd, stdout, stderr io.Writer) {
	cmd.Stdout = teeTo(cmd.Stdout, stdout)
	cmd.Stderr = teeTo(cmd.Stderr, stderr)
}

func teeTo(existing, extra io.Writer) io.Writer {
	if existing == nil {
		return extra
	}
	return io.MultiWriter(existing, extra)
}

// StdioTerminal runs commands on the given input and output, one at a time
// so concurrent scripts don't prompt over each other.
type StdioTerminal struct {
	mu     sync.Mutex
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
}

// NewStdioTerminal returns a terminal reading stdin and writing stdout and stderr.
func NewStdioTerminal(stdin io.Reader, stdout, stderr io.Writer) *StdioTerminal {
	return &StdioTerminal{stdin: stdin, stdout: stdout, stderr: stderr}
}

func (t *StdioTerminal) Run(ctx context.Context, cmd *exec.Cmd) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd.Stdin = t.stdin
	ShowOutput(cmd, t.stdout, t.stderr)
	return cmd.Run()
}
