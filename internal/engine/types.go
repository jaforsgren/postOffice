package engine

import (
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/protocol"
	"postOffice/internal/script"
)

// Call is one collection request to send with its scripts.
type Call struct {
	Item *postman.Item
	// Request is sent instead of Item.Request when set, e.g. a request with
	// unsaved edits or path parameters applied.
	Request *postman.Request
	// Scope supplies the scripts' collection, environment, folder path and
	// iteration, and the variables the request resolves against. A nil
	// Sender is replaced by the engine's pm.sendRequest sender.
	Scope script.RequestScope
}

// Result is what sending a request produced.
type Result struct {
	Protocol history.Protocol
	Response *http.Response
	// TestResult is nil for replays, which run no scripts.
	TestResult *script.TestResult
	// Cancelled is set when the context was cancelled before the send finished.
	Cancelled bool
}

// Execution is a request ready to send: its protocol is resolved, so callers
// can show its label before sending it, possibly from another goroutine.
type Execution struct {
	protocol protocol.Protocol
	request  *postman.Request
	// scope is nil for replays, which send the request as recorded.
	scope *script.RequestScope
}
