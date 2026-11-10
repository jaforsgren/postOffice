package tui

import (
	"testing"

	"postOffice/internal/postman"
)

func TestExecutorFor_RoutesByProtocol(t *testing.T) {
	m := NewModel(postman.NewParser())
	grpcItem := postman.Item{Request: &postman.Request{Method: "GRPC", URL: postman.URL{Raw: "grpc://localhost:1/svc.Service/Method"}}}
	httpItem := postman.Item{Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "http://localhost:1/"}}}

	if m.executorFor(grpcItem) != requestExecutor(m.grpcExecutor) {
		t.Error("gRPC items should use the gRPC executor")
	}
	if m.executorFor(httpItem) != requestExecutor(m.executor) {
		t.Error("HTTP items should use the HTTP executor")
	}
}
