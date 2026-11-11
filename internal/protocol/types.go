package protocol

import (
	"context"
	"errors"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

var (
	// ErrUnclaimedItem is returned when no registered protocol sends an item,
	// e.g. because it is a folder.
	ErrUnclaimedItem = errors.New("no protocol sends this item")
	// ErrUnknownProtocol is returned when a name, e.g. a history entry's, matches no registered protocol.
	ErrUnknownProtocol = errors.New("unknown request protocol")
)

// Protocol sends the requests of one transport. The TUI and the workflow
// runner send every request through the Protocol a Registry picks for it.
type Protocol interface {
	// Name identifies the protocol in history entries and logs. History is
	// persisted, so a protocol's name must never change.
	Name() history.Protocol
	// Claims reports whether item is a request this protocol sends.
	Claims(item *postman.Item) bool
	// Label is the short tag shown for a request in lists and status lines.
	Label(req *postman.Request) string
	// Execute sends req and gives up when ctx is cancelled. With a non-nil
	// scope the pre-request scripts run first (and may rewrite or skip the
	// request), then the test scripts if the protocol has a response to test.
	// Without a scope no scripts run, variables resolve req as given and the
	// result is nil.
	Execute(
		ctx context.Context,
		req *postman.Request,
		scope *script.RequestScope,
		variables []postman.VariableSource,
	) (*http.Response, *script.TestResult)
}

// scriptedExecutor sends a request and runs its scripts itself, as
// http.Executor and grpc.Executor do.
type scriptedExecutor interface {
	ExecuteContext(
		ctx context.Context,
		req *postman.Request,
		scope *script.RequestScope,
		variables []postman.VariableSource,
	) (*http.Response, *script.TestResult)
}

// messageSender sends a message built from req, resolving variables first,
// and gives up when ctx is cancelled.
type messageSender func(context.Context, *postman.Request, []postman.VariableSource) *http.Response
