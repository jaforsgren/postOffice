// Package engine is the single path every request takes, whether sent from
// the TUI, a workflow, a folder run or the headless CLI: protocol selection,
// variable resolution, scripts, transport, timing and cancellation.
package engine

import (
	"context"
	"errors"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/protocol"
	"postOffice/internal/script"
	"postOffice/internal/shell"
)

// Engine sends requests through the protocol a registry picks for them.
type Engine struct {
	protocols    *protocol.Registry
	scriptSender script.RequestSender
}

// New returns an engine for every built-in protocol. Scripts of any protocol
// send their pm.sendRequest calls with httpExecutor. Bash scripts run
// without input, so their prompts read EOF.
func New(httpExecutor *http.Executor) *Engine {
	return NewWithTerminal(httpExecutor, nil)
}

// NewWithTerminal is New with bash scripts attached to terminal, so they can prompt.
func NewWithTerminal(httpExecutor *http.Executor, terminal shell.Terminal) *Engine {
	return NewWithRegistry(protocol.NewBuiltinRegistry(httpExecutor, terminal), httpExecutor.SendRequest)
}

// NewWithRegistry returns an engine for the protocols of registry.
func NewWithRegistry(protocols *protocol.Registry, scriptSender script.RequestSender) *Engine {
	return &Engine{protocols: protocols, scriptSender: scriptSender}
}

// ScriptSender sends the pm.sendRequest calls of scripts that run outside a
// request, such as workflow scripts.
func (e *Engine) ScriptSender() script.RequestSender {
	return e.scriptSender
}

// Label is the short protocol tag shown for item in lists, e.g. "GET" or
// "ASB". ok is false when no protocol sends item, as for folders.
func (e *Engine) Label(item *postman.Item) (label string, ok bool) {
	requestProtocol, err := e.protocols.For(item)
	if err != nil {
		return "", false
	}
	return requestProtocol.Label(item.Request), true
}

// Prepare resolves the protocol that sends call. It fails with
// protocol.ErrUnclaimedItem when call.Item isn't a request.
func (e *Engine) Prepare(call Call) (*Execution, error) {
	requestProtocol, err := e.protocols.For(call.Item)
	if err != nil {
		return nil, err
	}
	request := call.Request
	if request == nil {
		request = call.Item.Request
	}
	scope := call.Scope
	if scope.Sender == nil {
		scope.Sender = e.scriptSender
	}
	return &Execution{protocol: requestProtocol, request: request, scope: &scope}, nil
}

// PrepareReplay prepares entry's recorded resolved request to be sent again
// as-is: no variables are resolved and no scripts run. It fails with
// protocol.ErrUnknownProtocol when entry's protocol isn't registered.
func (e *Engine) PrepareReplay(entry history.Entry) (*Execution, error) {
	requestProtocol, err := e.protocols.Named(entry.Protocol)
	if err != nil {
		return nil, err
	}
	return &Execution{protocol: requestProtocol, request: entry.Request()}, nil
}

// Protocol names the protocol that sends the execution.
func (x *Execution) Protocol() history.Protocol {
	return x.protocol.Name()
}

// Label is the short protocol tag of the request, e.g. "GET" or "ASB".
func (x *Execution) Label() string {
	return x.protocol.Label(x.request)
}

// Run sends the request and gives up when ctx is cancelled. Variables are
// read when it runs, so changes made since Prepare are seen.
func (x *Execution) Run(ctx context.Context) Result {
	var variables []postman.VariableSource
	if x.scope != nil {
		variables = x.scope.Variables()
	}
	response, testResult := x.protocol.Execute(ctx, x.request, x.scope, variables)
	return Result{
		Protocol:   x.protocol.Name(),
		Response:   response,
		TestResult: testResult,
		Cancelled:  errors.Is(ctx.Err(), context.Canceled),
	}
}
