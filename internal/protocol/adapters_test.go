package protocol

import (
	"context"
	"errors"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"testing"
)

const (
	testProtocolName   history.Protocol = "TEST"
	sentStatusCode                      = 202
	executedStatusCode                  = 204
)

// recordingSender records what a messageProtocol hands its send function.
type recordingSender struct {
	calls     int
	request   *postman.Request
	variables []postman.VariableSource
}

func (s *recordingSender) send(_ context.Context, req *postman.Request, variables []postman.VariableSource) *http.Response {
	s.calls++
	s.request = req
	s.variables = variables
	return &http.Response{StatusCode: sentStatusCode}
}

// recordingExecutor records the arguments a scriptedProtocol forwards.
type recordingExecutor struct {
	ctx   context.Context
	scope *script.RequestScope
}

func (e *recordingExecutor) ExecuteContext(
	ctx context.Context,
	_ *postman.Request,
	scope *script.RequestScope,
	_ []postman.VariableSource,
) (*http.Response, *script.TestResult) {
	e.ctx = ctx
	e.scope = scope
	return &http.Response{StatusCode: executedStatusCode}, &script.TestResult{}
}

func unusedScriptSender(script.SendRequestSpec) (*script.ResponseData, error) {
	return nil, nil
}

func preRequestItem(lines ...string) *postman.Item {
	return &postman.Item{
		Name:    "message",
		Request: &postman.Request{Method: "POST", URL: postman.URL{Raw: "asb://{{queue}}"}},
		Events:  []postman.Event{{Listen: string(script.PhasePreRequest), Script: postman.Script{Exec: lines}}},
	}
}

func variableValue(variables []postman.VariableSource, key string) string {
	for _, variable := range variables {
		if variable.Key == key {
			return variable.Value
		}
	}
	return ""
}

func TestMessageProtocol_WithoutScopeSendsVariablesAsGiven(t *testing.T) {
	sender := &recordingSender{}
	protocol := newMessageProtocol(testProtocolName, (*postman.Item).IsServiceBus, sender.send, nil)
	req := &postman.Request{URL: postman.URL{Raw: "asb://orders"}}
	variables := []postman.VariableSource{{Key: "queue", Value: "orders"}}

	resp, result := protocol.Execute(context.Background(), req, nil, variables)

	if resp.StatusCode != sentStatusCode || result != nil {
		t.Errorf("Execute() = %+v, %+v, want the sent response and no script result", resp, result)
	}
	if sender.request != req || variableValue(sender.variables, "queue") != "orders" {
		t.Errorf("sent %+v with %+v, want the request and variables as given", sender.request, sender.variables)
	}
}

func TestMessageProtocol_SendsWithVariablesSetByPreRequestScript(t *testing.T) {
	sender := &recordingSender{}
	protocol := newMessageProtocol(testProtocolName, (*postman.Item).IsServiceBus, sender.send, unusedScriptSender)
	item := preRequestItem("pm.collectionVariables.set('queue', 'generated');")
	scope := &script.RequestScope{Collection: &postman.Collection{}, Item: item}

	_, result := protocol.Execute(context.Background(), item.Request, scope, nil)

	if result == nil || len(result.Errors) > 0 {
		t.Fatalf("pre-request result = %+v, want a clean run", result)
	}
	if got := variableValue(sender.variables, "queue"); got != "generated" {
		t.Errorf("queue = %q, want the value the pre-request script set", got)
	}
	if scope.Sender == nil {
		t.Error("expected the scope to get a pm.sendRequest sender")
	}
}

func TestMessageProtocol_SkippedByPreRequestScriptSendsNothing(t *testing.T) {
	sender := &recordingSender{}
	protocol := newMessageProtocol(testProtocolName, (*postman.Item).IsServiceBus, sender.send, nil)
	item := preRequestItem("pm.execution.skipRequest();")

	resp, result := protocol.Execute(context.Background(), item.Request, &script.RequestScope{Item: item}, nil)

	if sender.calls != 0 {
		t.Errorf("sent %d message(s), want none", sender.calls)
	}
	if !resp.Skipped || resp.Status != http.SkippedStatus || result == nil || !result.Skipped {
		t.Errorf("Execute() = %+v, %+v, want a skipped response", resp, result)
	}
}

func TestMessageProtocol_PreRequestErrorsDontStopTheSend(t *testing.T) {
	sender := &recordingSender{}
	protocol := newMessageProtocol(testProtocolName, (*postman.Item).IsServiceBus, sender.send, nil)
	item := preRequestItem("throw new Error('boom');")

	_, result := protocol.Execute(context.Background(), item.Request, &script.RequestScope{Item: item}, nil)

	if sender.calls != 1 || result == nil || len(result.Errors) == 0 {
		t.Errorf("sent %d message(s) with result %+v, want one send and the script error", sender.calls, result)
	}
}

const respondedBody = `[{"subject":"LF-1"}]`

func respondingSender(calls *int) messageSender {
	return func(context.Context, *postman.Request, []postman.VariableSource) *http.Response {
		*calls++
		return &http.Response{StatusCode: sentStatusCode, Status: "1 message(s) peeked", Body: respondedBody}
	}
}

func peekItem(events ...postman.Event) *postman.Item {
	return &postman.Item{
		Name:    "peek",
		Request: &postman.Request{Method: postman.MethodServiceBusPeek, URL: postman.URL{Raw: "asb://payments/tap"}},
		Events:  events,
	}
}

func scriptEvent(phase script.Phase, lines ...string) postman.Event {
	return postman.Event{Listen: string(phase), Script: postman.Script{Exec: lines}}
}

func TestRespondingProtocol_TestsTheResponse(t *testing.T) {
	calls := 0
	protocol := newRespondingProtocol(testProtocolName, (*postman.Item).IsServiceBusPeek, postman.MethodServiceBusPeek, respondingSender(&calls), unusedScriptSender)
	item := peekItem(scriptEvent(script.PhaseTest, "pm.test('one message', () => pm.expect(pm.response.json()[0].subject).to.eql('LF-1'));"))

	resp, result := protocol.Execute(context.Background(), item.Request, &script.RequestScope{Item: item}, nil)

	if calls != 1 || resp.Body != respondedBody {
		t.Fatalf("sent %d time(s), response %+v", calls, resp)
	}
	if result == nil || len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Errorf("result = %+v, want the passing test", result)
	}
}

func TestRespondingProtocol_PreRequestErrorStopsTheSend(t *testing.T) {
	calls := 0
	protocol := newRespondingProtocol(testProtocolName, (*postman.Item).IsServiceBusPeek, postman.MethodServiceBusPeek, respondingSender(&calls), nil)
	item := peekItem(scriptEvent(script.PhasePreRequest, "throw new Error('boom');"))

	resp, _ := protocol.Execute(context.Background(), item.Request, &script.RequestScope{Item: item}, nil)

	if calls != 0 || !errors.Is(resp.Error, http.ErrPreRequestScript) {
		t.Errorf("sent %d time(s) with error %v, want no send and ErrPreRequestScript", calls, resp.Error)
	}
}

func TestRespondingProtocol_SkippedByPreRequestScript(t *testing.T) {
	calls := 0
	protocol := newRespondingProtocol(testProtocolName, (*postman.Item).IsServiceBusPeek, postman.MethodServiceBusPeek, respondingSender(&calls), nil)
	item := peekItem(scriptEvent(script.PhasePreRequest, "pm.execution.skipRequest();"))

	resp, result := protocol.Execute(context.Background(), item.Request, &script.RequestScope{Item: item}, nil)

	if calls != 0 || !resp.Skipped || !result.Skipped {
		t.Errorf("sent %d time(s), response %+v, want a skipped peek", calls, resp)
	}
}

func TestRespondingProtocol_WithoutScopeSendsAsGiven(t *testing.T) {
	calls := 0
	protocol := newRespondingProtocol(testProtocolName, (*postman.Item).IsServiceBusPeek, postman.MethodServiceBusPeek, respondingSender(&calls), nil)

	resp, result := protocol.Execute(context.Background(), peekItem().Request, nil, nil)

	if calls != 1 || resp.Body != respondedBody || result != nil {
		t.Errorf("Execute() = %+v, %+v, want the response and no script result", resp, result)
	}
}

func TestScriptedProtocol_ForwardsToExecutor(t *testing.T) {
	executor := &recordingExecutor{}
	protocol := newScriptedProtocol(testProtocolName, (*postman.Item).IsRequest, executor)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scope := &script.RequestScope{}

	resp, result := protocol.Execute(ctx, &postman.Request{}, scope, nil)

	if resp.StatusCode != executedStatusCode || result == nil {
		t.Errorf("Execute() = %+v, %+v, want the executor's result", resp, result)
	}
	if executor.ctx != ctx || executor.scope != scope {
		t.Error("expected the context and scope to reach the executor")
	}
}
