package protocol

import (
	"context"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

// scriptedProtocol adapts an executor that runs scripts itself (HTTP, gRPC).
type scriptedProtocol struct {
	name     history.Protocol
	claims   func(*postman.Item) bool
	executor scriptedExecutor
}

func newScriptedProtocol(name history.Protocol, claims func(*postman.Item) bool, executor scriptedExecutor) Protocol {
	return scriptedProtocol{name: name, claims: claims, executor: executor}
}

func (p scriptedProtocol) Name() history.Protocol {
	return p.name
}

func (p scriptedProtocol) Claims(item *postman.Item) bool {
	return p.claims(item)
}

func (p scriptedProtocol) Label(req *postman.Request) string {
	return req.Method
}

func (p scriptedProtocol) Execute(
	ctx context.Context,
	req *postman.Request,
	scope *script.RequestScope,
	variables []postman.VariableSource,
) (*http.Response, *script.TestResult) {
	return p.executor.ExecuteContext(ctx, req, scope, variables)
}
