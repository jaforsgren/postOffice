package script

import (
	"errors"
	"strings"
	"testing"
	"time"

	"postOffice/internal/postman"
)

func TestSetTimeout_RunsCallbacksInDueOrderAfterTheScript(t *testing.T) {
	ctx := &ExecutionContext{}

	result := NewRuntime().ExecutePreRequestScript(jsScript(
		"const order = [];",
		"setTimeout(() => { order.push('late'); pm.collectionVariables.set('order', order.join(',')); }, 20);",
		"setTimeout((tag) => order.push(tag), 5, 'early');",
		"const cancelled = setTimeout(() => order.push('cancelled'), 1);",
		"clearTimeout(cancelled);",
		"order.push('sync');",
	), ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(ctx.CollectionVars) != 1 || ctx.CollectionVars[0].Value != "sync,early,late" {
		t.Errorf("expected sync,early,late, got %+v", ctx.CollectionVars)
	}
}

func TestSetTimeout_CallbackErrorIsReportedAndOthersStillRun(t *testing.T) {
	ctx := &ExecutionContext{}

	result := NewRuntime().ExecutePreRequestScript(jsScript(
		"setTimeout(() => { throw new Error('callback failed'); }, 0);",
		"setTimeout(() => pm.collectionVariables.set('after', 'ran'), 1);",
	), ctx)

	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "callback failed") {
		t.Errorf("expected the callback error, got %v", result.Errors)
	}
	if len(ctx.CollectionVars) != 1 {
		t.Error("expected the second callback to run")
	}
}

func TestTimeout_AppliesPerSegmentNotToWaiting(t *testing.T) {
	const segmentTimeout = 100 * time.Millisecond

	waiting := NewRuntimeWithTimeout(segmentTimeout).ExecutePreRequestScript(jsScript(
		"setTimeout(() => {}, 150);",
	), &ExecutionContext{})
	if len(waiting.Errors) > 0 {
		t.Errorf("expected waiting on a timer not to count against the timeout, got %v", waiting.Errors)
	}

	spinning := NewRuntimeWithTimeout(segmentTimeout).ExecutePreRequestScript(jsScript(
		"setTimeout(() => { while (true) {} }, 0);",
	), &ExecutionContext{})
	if len(spinning.Errors) != 1 || !strings.Contains(spinning.Errors[0], "timeout") {
		t.Errorf("expected a timeout from the spinning callback, got %v", spinning.Errors)
	}
}

func TestTimeout_SendRequestWaitDoesNotCountAgainstTheSegment(t *testing.T) {
	const segmentTimeout = 100 * time.Millisecond
	const slowServerDelay = 150 * time.Millisecond
	ctx := &ExecutionContext{SendRequest: func(SendRequestSpec) (*ResponseData, error) {
		time.Sleep(slowServerDelay)
		return &ResponseData{StatusCode: 200}, nil
	}}

	result := NewRuntimeWithTimeout(segmentTimeout).ExecutePreRequestScript(jsScript(
		"pm.sendRequest('https://api.example.com/first', () => {",
		"  pm.sendRequest('https://api.example.com/second', (err, res) => pm.collectionVariables.set('code', String(res.code)));",
		"});",
	), ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("expected slow HTTP not to count against the timeout, got %v", result.Errors)
	}
	if len(ctx.CollectionVars) != 1 || ctx.CollectionVars[0].Value != "200" {
		t.Errorf("expected the nested callback to run, got %+v", ctx.CollectionVars)
	}
}

func TestSendRequest_CallbackRunsAfterScriptWithResponse(t *testing.T) {
	var sent SendRequestSpec
	ctx := &ExecutionContext{
		Response: &ResponseData{StatusCode: 200},
		SendRequest: func(spec SendRequestSpec) (*ResponseData, error) {
			sent = spec
			return &ResponseData{StatusCode: 201, Body: `{"phoneNumber":"0701"}`, Headers: map[string][]string{"Content-Type": {"application/json"}}}, nil
		},
	}

	result := NewRuntime().ExecuteTestScript(jsScript(
		"let order = 'script';",
		"pm.sendRequest({",
		"  url: 'https://api.example.com/User',",
		"  method: 'PUT',",
		"  header: { authKey: 'anything' },",
		"  body: { mode: 'raw', raw: JSON.stringify({ phoneNumber: '0701' }) },",
		"}, (err, res) => {",
		"  order += ',callback';",
		"  pm.test('no error', () => pm.expect(err).to.be.null);",
		"  pm.test('status', () => pm.expect(res.code).to.eql(201));",
		"  pm.test('json', () => pm.expect(res.json().phoneNumber).to.eql('0701'));",
		"  pm.test('headers', () => pm.expect(res.headers.get('content-type')).to.eql('application/json'));",
		"  pm.test('order', () => pm.expect(order).to.eql('script,end,callback'));",
		"});",
		"order += ',end';",
	), ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(result.Tests) != 5 {
		t.Fatalf("expected 5 tests from the callback, got %d", len(result.Tests))
	}
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
	want := SendRequestSpec{Method: "PUT", URL: "https://api.example.com/User", Headers: []postman.Header{{Key: "authKey", Value: "anything"}}, Body: `{"phoneNumber":"0701"}`}
	if sent.Method != want.Method || sent.URL != want.URL || sent.Body != want.Body || len(sent.Headers) != 1 || sent.Headers[0] != want.Headers[0] {
		t.Errorf("sent %+v, want %+v", sent, want)
	}
}

func TestSendRequest_StringURLAndHeaderListForms(t *testing.T) {
	var specs []SendRequestSpec
	ctx := &ExecutionContext{SendRequest: func(spec SendRequestSpec) (*ResponseData, error) {
		specs = append(specs, spec)
		return &ResponseData{StatusCode: 200}, nil
	}}

	result := NewRuntime().ExecutePreRequestScript(jsScript(
		"pm.sendRequest('https://example.com/ping', () => {});",
		"pm.sendRequest({ url: 'https://example.com/x', header: [{ key: 'A', value: '1' }] }, () => {});",
	), ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	if len(specs) != 2 || specs[0].Method != "GET" || specs[0].URL != "https://example.com/ping" {
		t.Fatalf("unexpected string-form spec %+v", specs)
	}
	if len(specs[1].Headers) != 1 || specs[1].Headers[0].Key != "A" || specs[1].Headers[0].Value != "1" {
		t.Errorf("unexpected header-list spec %+v", specs[1])
	}
}

func TestSendRequest_NestedCallbacksAndPromiseForm(t *testing.T) {
	calls := 0
	ctx := &ExecutionContext{SendRequest: func(SendRequestSpec) (*ResponseData, error) {
		calls++
		return &ResponseData{StatusCode: 200 + calls}, nil
	}}

	result := NewRuntime().ExecutePreRequestScript(jsScript(
		"pm.sendRequest('https://a', (e1, first) => {",
		"  pm.sendRequest('https://b', (e2, second) => pm.collectionVariables.set('codes', `${first.code},${second.code}`));",
		"});",
		"(async () => { const third = await pm.sendRequest('https://c'); pm.collectionVariables.set('awaited', String(third.code)); })();",
	), ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	values := map[string]string{}
	for _, v := range ctx.CollectionVars {
		values[v.Key] = v.Value
	}
	if values["codes"] != "201,203" {
		t.Errorf("expected nested callback codes 201,203, got %q", values["codes"])
	}
	if values["awaited"] != "202" {
		t.Errorf("expected awaited code 202, got %q", values["awaited"])
	}
}

func TestSendRequest_ErrorsReachTheCallback(t *testing.T) {
	errUnreachable := errors.New("connection refused")
	cases := map[string]RequestSender{
		"sender error": func(SendRequestSpec) (*ResponseData, error) { return nil, errUnreachable },
		"no sender":    nil,
	}
	for name, sender := range cases {
		ctx := &ExecutionContext{SendRequest: sender}
		result := NewRuntime().ExecutePreRequestScript(jsScript(
			"pm.sendRequest('https://x', (err, res) => {",
			"  pm.test('err set', () => pm.expect(err).to.exist);",
			"  pm.test('no response', () => pm.expect(res).to.be.null);",
			"});",
		), ctx)
		if len(result.Errors) > 0 || len(result.Tests) != 2 {
			t.Fatalf("%s: unexpected result %+v", name, result)
		}
		for _, test := range result.Tests {
			if !test.Passed {
				t.Errorf("%s/%s: %s", name, test.Name, test.Error)
			}
		}
	}
}
