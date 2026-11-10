package grpc

import (
	"testing"

	"postOffice/internal/postman"
)

func TestInvoke_ResolvesVariablesIntoResponse(t *testing.T) {
	req := &postman.Request{
		Method: MethodLabel,
		URL:    postman.URL{Raw: "{{target}}"},
		Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: `{"id":"{{id}}"}`},
	}
	variables := []postman.VariableSource{
		{Key: "target", Value: "not-a-grpc-url"},
		{Key: "id", Value: "42"},
	}

	resp := Invoke(req, variables)

	if resp.RequestMethod != MethodLabel {
		t.Errorf("RequestMethod = %q, want %q", resp.RequestMethod, MethodLabel)
	}
	if resp.RequestURL != "not-a-grpc-url" {
		t.Errorf("RequestURL = %q, want resolved URL", resp.RequestURL)
	}
	if resp.RequestBody != `{"id":"42"}` {
		t.Errorf("RequestBody = %q, want resolved body", resp.RequestBody)
	}
	if resp.Error == nil {
		t.Error("Error = nil, want invalid gRPC URL error")
	}
	if req.URL.Raw != "{{target}}" || req.Body.Raw != `{"id":"{{id}}"}` {
		t.Error("Invoke mutated the original request")
	}
}

func TestInvoke_WithoutBody(t *testing.T) {
	resp := Invoke(&postman.Request{URL: postman.URL{Raw: "grpc://"}}, nil)

	if resp.RequestBody != "" {
		t.Errorf("RequestBody = %q, want empty", resp.RequestBody)
	}
	if resp.Error == nil {
		t.Error("Error = nil, want invalid gRPC URL error")
	}
}
