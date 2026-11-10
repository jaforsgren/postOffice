package workflow

import (
	"postOffice/internal/postman"
	"testing"
)

func TestExecuteGRPC_FailureReturnsViewableResponse(t *testing.T) {
	runner := NewRunner(buildTestCollection(), nil, nil)
	item := &postman.Item{
		Name: "Broken",
		Request: &postman.Request{
			URL:  postman.URL{Raw: "grpc://missing-service-and-method"},
			Body: &postman.Body{Raw: "{}"},
		},
	}

	stepResp, resp := runner.executeGRPC(item, nil)

	if stepResp.Code != 0 {
		t.Errorf("step code = %d, want 0 for a failed call", stepResp.Code)
	}
	if resp.Error == nil {
		t.Fatal("expected response error")
	}
	if resp.RequestMethod != grpcRequestMethod {
		t.Errorf("RequestMethod = %q, want %q", resp.RequestMethod, grpcRequestMethod)
	}
	if resp.RequestBody != "{}" {
		t.Errorf("RequestBody = %q, want resolved body", resp.RequestBody)
	}
	if resp.Duration <= 0 || stepResp.Time != resp.Duration.Milliseconds() {
		t.Errorf("durations disagree: step=%dms response=%v", stepResp.Time, resp.Duration)
	}
}
