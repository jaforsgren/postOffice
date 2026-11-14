package protocol

import (
	"context"
	"fmt"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

// respondingProtocol adapts a transport that answers each request but runs
// no scripts itself (Service Bus peeks). As for HTTP, pre-request script
// errors stop the send and the test scripts run against a successful response.
type respondingProtocol struct {
	name   history.Protocol
	claims func(*postman.Item) bool
	label  string
	send   messageSender
	// scriptSender sends the pm.sendRequest calls of the request's scripts.
	scriptSender script.RequestSender
}

func newRespondingProtocol(
	name history.Protocol,
	claims func(*postman.Item) bool,
	label string,
	send messageSender,
	scriptSender script.RequestSender,
) Protocol {
	return respondingProtocol{name: name, claims: claims, label: label, send: send, scriptSender: scriptSender}
}

func (p respondingProtocol) Name() history.Protocol {
	return p.name
}

func (p respondingProtocol) Claims(item *postman.Item) bool {
	return p.claims(item)
}

func (p respondingProtocol) Label(*postman.Request) string {
	return p.label
}

func (p respondingProtocol) Execute(
	ctx context.Context,
	req *postman.Request,
	scope *script.RequestScope,
	variables []postman.VariableSource,
) (*http.Response, *script.TestResult) {
	if scope == nil {
		return p.send(ctx, req, variables), nil
	}
	if scope.Sender == nil {
		scope.Sender = p.scriptSender
	}
	scriptRun := script.NewRequestRun(*scope)
	request, result := scriptRun.PreRequest(req)
	if len(result.Errors) > 0 {
		return &http.Response{Error: fmt.Errorf("%w: %v", http.ErrPreRequestScript, result.Errors)}, result
	}
	if result.Skipped {
		return skippedResponse(), result
	}

	// Variables are read after the scripts, which may have set some.
	resp := p.send(ctx, request, scope.Variables())
	if resp.Error == nil {
		result.Merge(scriptRun.Test(script.NewRequestData(request), &script.ResponseData{
			StatusCode:   resp.StatusCode,
			Status:       resp.Status,
			Body:         resp.Body,
			ResponseTime: resp.Duration.Milliseconds(),
		}))
	}
	return resp, result
}

// skippedResponse stands for a request its pre-request scripts skipped.
func skippedResponse() *http.Response {
	return &http.Response{Skipped: true, Status: http.SkippedStatus}
}
