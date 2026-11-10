package script

import (
	"testing"

	"postOffice/internal/postman"
)

func TestURLPath(t *testing.T) {
	cases := map[string]string{
		"{{SAPI_BASE}}/api/v1/conversation/123?x=1": "/api/v1/conversation/123",
		"https://example.com/a/b#frag":              "/a/b",
		"https://example.com":                       "/",
		"{{base}}":                                  "/",
		"/relative/path":                            "/relative/path",
	}
	for raw, want := range cases {
		if got := urlPath(raw); got != want {
			t.Errorf("urlPath(%q) = %q, want %q", raw, got, want)
		}
	}
}

func runPreRequestScript(t *testing.T, ctx *ExecutionContext, lines ...string) *TestResult {
	t.Helper()
	result := NewRuntime().ExecutePreRequestScript(jsScript(lines...), ctx)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected script errors: %v", result.Errors)
	}
	return result
}

func TestPmRequest_ReadsUnresolvedValues(t *testing.T) {
	ctx := &ExecutionContext{Request: &RequestData{
		Method:  "PUT",
		URL:     "{{SAPI_BASE}}/api/v1/conversation/1",
		Headers: []postman.Header{{Key: "x-sejfa-auth", Value: "{{sejfaToken}}"}},
		Body:    `{"phoneNumber":"0701"}`,
	}}

	result := runPreRequestScript(t, ctx,
		"pm.test('header', () => pm.expect(pm.request.headers.get('X-Sejfa-Auth')).to.eql('{{sejfaToken}}'));",
		"pm.test('missing header', () => pm.expect(pm.request.headers.get('nope')).to.be.undefined);",
		"pm.test('method', () => pm.expect(pm.request.method).to.eql('PUT'));",
		"pm.test('path', () => pm.expect(pm.request.url.getPath()).to.eql('/api/v1/conversation/1'));",
		"pm.test('url string', () => pm.expect(`${pm.request.url}`).to.eql('{{SAPI_BASE}}/api/v1/conversation/1'));",
		"pm.test('body', () => pm.expect(JSON.parse(pm.request.body.raw).phoneNumber).to.eql('0701'));",
	)

	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
}

func TestPmRequest_Mutations(t *testing.T) {
	ctx := &ExecutionContext{Request: &RequestData{
		Headers: []postman.Header{{Key: "Accept", Value: "text/plain"}, {Key: "X-Remove", Value: "1"}},
		Body:    "old",
	}}

	runPreRequestScript(t, ctx,
		"pm.request.headers.upsert({ key: 'accept', value: 'application/json' });",
		"pm.request.headers.add({ key: 'X-Added', value: 'yes' });",
		"pm.request.headers.remove('x-remove');",
		"pm.request.body.update({ mode: 'raw', raw: 'from options' });",
	)

	want := []postman.Header{{Key: "Accept", Value: "application/json"}, {Key: "X-Added", Value: "yes"}}
	if len(ctx.Request.Headers) != len(want) || ctx.Request.Headers[0] != want[0] || ctx.Request.Headers[1] != want[1] {
		t.Errorf("headers = %+v, want %+v", ctx.Request.Headers, want)
	}
	if ctx.Request.Body != "from options" {
		t.Errorf("body = %q, want %q", ctx.Request.Body, "from options")
	}
}

func TestPmRequest_BodyUpdateRejectsNonRawModes(t *testing.T) {
	ctx := &ExecutionContext{Request: &RequestData{}}
	result := NewRuntime().ExecutePreRequestScript(jsScript("pm.request.body.update({ mode: 'formdata', formdata: [] });"), ctx)

	if len(result.Errors) != 1 || !contains(result.Errors[0], "only supports raw bodies") {
		t.Errorf("expected raw-mode error, got %v", result.Errors)
	}
}

func TestPmInfo(t *testing.T) {
	ctx := &ExecutionContext{Info: RequestInfo{RequestName: "get claim", RequestID: "claims/get claim"}}

	result := runPreRequestScript(t, ctx,
		"pm.test('name', () => pm.expect(pm.info.requestName).to.eql('get claim'));",
		"pm.test('id', () => pm.expect(pm.info.requestId).to.eql('claims/get claim'));",
		"pm.test('event', () => pm.expect(pm.info.eventName).to.eql('prerequest'));",
	)

	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
}

func TestPmExecution_SkipRequestOnlyAppliesBeforeSending(t *testing.T) {
	skipScript := jsScript("pm.execution.skipRequest();")

	if pre := NewRuntime().ExecutePreRequestScript(skipScript, &ExecutionContext{}); !pre.Skipped {
		t.Error("expected skipRequest to skip in a pre-request script")
	}
	if test := NewRuntime().ExecuteTestScript(skipScript, &ExecutionContext{Response: &ResponseData{}}); test.Skipped {
		t.Error("expected skipRequest to be ignored in a test script")
	}
}

func TestPmExecution_SetNextRequest(t *testing.T) {
	cases := map[string]NextRequest{
		"pm.execution.setNextRequest('get policy by id');": {Set: true, Name: "get policy by id"},
		"pm.execution.setNextRequest(null);":               {Set: true},
		"":                                                 {},
	}
	for code, want := range cases {
		result := NewRuntime().ExecuteTestScript(jsScript(code, "// keep script non-empty"), &ExecutionContext{Response: &ResponseData{}})
		if result.NextRequest != want {
			t.Errorf("%q: NextRequest = %+v, want %+v", code, result.NextRequest, want)
		}
	}
}

func TestVariableAPIs_HasAndUnset(t *testing.T) {
	ctx := &ExecutionContext{
		CollectionVars:  []postman.Variable{{Key: "attempt", Value: "3"}},
		EnvironmentVars: []postman.EnvVariable{{Key: "token", Value: "t"}},
	}

	result := runPreRequestScript(t, ctx,
		"pm.test('has', () => pm.expect(pm.collectionVariables.has('attempt')).to.be.true);",
		"pm.collectionVariables.unset('attempt');",
		"pm.environment.unset('token');",
		"pm.test('gone', () => pm.expect(pm.variables.get('attempt')).to.be.undefined);",
	)

	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
	if len(ctx.CollectionVars) != 0 || len(ctx.EnvironmentVars) != 0 {
		t.Errorf("expected variables removed, got %+v %+v", ctx.CollectionVars, ctx.EnvironmentVars)
	}
}

func TestPmResponse_SizeAndHeaderList(t *testing.T) {
	ctx := &ExecutionContext{Response: &ResponseData{
		StatusCode: 200,
		Body:       "12345",
		Headers:    map[string][]string{"Content-Disposition": {"attachment"}, "Content-Type": {"application/pdf"}},
	}}

	result := NewRuntime().ExecuteTestScript(jsScript(
		"pm.test('size', () => pm.expect(pm.response.responseSize).to.eql(5));",
		"pm.test('has', () => pm.expect(pm.response.headers.has('content-disposition')).to.be.true);",
		"pm.test('toObject', () => pm.expect(pm.response.headers.toObject()).to.eql({ 'Content-Disposition': 'attachment', 'Content-Type': 'application/pdf' }));",
		"pm.test('no mutation', () => pm.expect(pm.response.headers.add).to.be.undefined);",
	), ctx)

	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
}

func TestRequestData_ApplyTo(t *testing.T) {
	original := &postman.Request{Method: "GET", URL: postman.URL{Raw: "{{base}}/a", Host: []string{"{{base}}"}}}

	t.Run("unchanged URL keeps its structured form", func(t *testing.T) {
		updated := NewRequestData(original).ApplyTo(original)
		if len(updated.URL.Host) != 1 {
			t.Errorf("expected host parts kept, got %+v", updated.URL)
		}
		if updated.Body != nil {
			t.Error("expected no body to be invented")
		}
	})

	t.Run("body added to a bodyless request", func(t *testing.T) {
		data := NewRequestData(original)
		data.Body = "{}"
		if updated := data.ApplyTo(original); updated.Body == nil || updated.Body.Mode != rawBodyMode || updated.Body.Raw != "{}" {
			t.Errorf("expected raw body, got %+v", updated.Body)
		}
	})
}
