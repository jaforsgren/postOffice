package protocol

import (
	"context"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

// messageProtocol adapts a one-way message send (Service Bus, AMQP). A broker
// returns nothing to test, so only pre-request scripts run, and unlike HTTP
// their errors don't stop the send.
type messageProtocol struct {
	name   history.Protocol
	claims func(*postman.Item) bool
	send   messageSender
	// scriptSender sends the pm.sendRequest calls of a message's scripts.
	scriptSender script.RequestSender
}

func newMessageProtocol(
	name history.Protocol,
	claims func(*postman.Item) bool,
	send messageSender,
	scriptSender script.RequestSender,
) Protocol {
	return messageProtocol{name: name, claims: claims, send: send, scriptSender: scriptSender}
}

func (p messageProtocol) Name() history.Protocol {
	return p.name
}

func (p messageProtocol) Claims(item *postman.Item) bool {
	return p.claims(item)
}

func (p messageProtocol) Label(*postman.Request) string {
	return string(p.name)
}

func (p messageProtocol) Execute(
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
	request, preResult := script.NewRequestRun(*scope).PreRequest(req)
	if preResult.Skipped {
		return &http.Response{Skipped: true, Status: http.SkippedStatus}, preResult
	}
	// Variables are read after the scripts, which may have set some.
	return p.send(ctx, request, scope.Variables()), preResult
}
