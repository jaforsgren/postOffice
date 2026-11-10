package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"postOffice/internal/postman"
	"postOffice/internal/timing"
)

const testCallTimeout = 5 * time.Second

func executeTestRequest(t *testing.T, rawURL string) (*Result, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), testCallTimeout)
	defer cancel()
	return ExecuteRequest(ctx, &postman.Request{URL: postman.URL{Raw: rawURL}}, "")
}

func TestExecuteRequest_ReportsReflectAndInvokePhases(t *testing.T) {
	addr := startHealthServer(t, withReflection).endpoint

	result, err := executeTestRequest(t, "grpc://"+addr+"/grpc.health.v1.Health/Check")
	if err != nil {
		t.Fatalf("ExecuteRequest: %v", err)
	}
	if !strings.Contains(result.Body, "SERVING") {
		t.Errorf("Body = %q, want serving status", result.Body)
	}

	if len(result.Phases) != 2 {
		t.Fatalf("got %d phases, want 2: %+v", len(result.Phases), result.Phases)
	}
	reflect, invoke := result.Phases[0], result.Phases[1]
	if reflect.Name != PhaseReflect || invoke.Name != PhaseInvoke {
		t.Errorf("phase names = [%q %q], want [%q %q]", reflect.Name, invoke.Name, PhaseReflect, PhaseInvoke)
	}
	for _, phase := range result.Phases {
		if phase.Status != timing.StatusMeasured {
			t.Errorf("phase %q status = %v, want measured", phase.Name, phase.Status)
		}
	}
	if invoke.Offset != reflect.Offset+reflect.Duration {
		t.Errorf("invoke should start when reflection ends: %+v", result.Phases)
	}
	if end := invoke.Offset + invoke.Duration; end > result.Duration {
		t.Errorf("phases end at %v, beyond total %v", end, result.Duration)
	}
}

func TestExecuteRequest_UnknownServiceMeasuresReflectOnly(t *testing.T) {
	addr := startHealthServer(t, withReflection).endpoint

	result, err := executeTestRequest(t, "grpc://"+addr+"/no.such.Service/Method")
	if err == nil {
		t.Fatal("expected reflection error")
	}
	if len(result.Phases) != 1 || result.Phases[0].Name != PhaseReflect {
		t.Errorf("phases = %+v, want only %q", result.Phases, PhaseReflect)
	}
}

func TestExecuteRequest_InvalidURLHasNoPhases(t *testing.T) {
	result, err := executeTestRequest(t, "http://not-grpc")
	if err == nil {
		t.Fatal("expected invalid URL error")
	}
	if len(result.Phases) != 0 {
		t.Errorf("phases = %+v, want none", result.Phases)
	}
	if result.Duration <= 0 {
		t.Error("expected total duration to be recorded")
	}
}
