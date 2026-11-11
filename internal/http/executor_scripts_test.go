package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/script"
)

func prerequest(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhasePreRequest), Script: postman.Script{Exec: lines}}
}

func testEvent(lines ...string) postman.Event {
	return postman.Event{Listen: string(script.PhaseTest), Script: postman.Script{Exec: lines}}
}

func scopeFor(collection *postman.Collection, item *postman.Item) *script.RequestScope {
	return &script.RequestScope{Collection: collection, Item: item}
}

func TestExecute_PreRequestMutationsReachTheServer(t *testing.T) {
	var gotBody, gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		gotHeader = r.Header.Get("X-Trace")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := &postman.Request{Method: "POST", URL: postman.URL{Raw: "{{base}}/offers"}, Body: &postman.Body{Mode: "raw", Raw: "original"}}
	collection := &postman.Collection{
		Variables: []postman.Variable{{Key: "base", Value: server.URL}},
		Events:    []postman.Event{prerequest("pm.request.headers.upsert({ key: 'X-Trace', value: pm.info.requestName });")},
	}
	item := &postman.Item{Name: "create offer", Request: req, Events: []postman.Event{prerequest("pm.request.body.update('rewritten');")}}

	resp, result := NewExecutor().ExecuteContext(context.Background(), req, scopeFor(collection, item), nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected script errors: %v", result.Errors)
	}
	if gotBody != "rewritten" || gotHeader != "create offer" {
		t.Errorf("server got body %q header %q", gotBody, gotHeader)
	}
	if req.Body.Raw != "original" {
		t.Error("expected the collection's request to stay untouched")
	}
}

func TestExecute_SkipRequestSendsNothing(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()

	req := &postman.Request{Method: "GET", URL: postman.URL{Raw: server.URL}}
	item := &postman.Item{Name: "guarded", Request: req, Events: []postman.Event{
		prerequest("console.warn('missing policy_id'); pm.execution.skipRequest();"),
		testEvent("pm.test('must not run', () => {});"),
	}}

	resp, result := NewExecutor().ExecuteContext(context.Background(), req, scopeFor(&postman.Collection{}, item), nil)

	if hits.Load() != 0 {
		t.Errorf("expected no request to be sent, got %d", hits.Load())
	}
	if !resp.Skipped || resp.Status != SkippedStatus || resp.Error != nil {
		t.Errorf("expected a skipped response, got %+v", resp)
	}
	if len(result.Tests) != 0 {
		t.Error("expected test scripts not to run for a skipped request")
	}
	if len(result.ConsoleLogs) != 1 {
		t.Errorf("expected the skip reason in the console log, got %v", result.ConsoleLogs)
	}
}

func TestExecute_PreRequestErrorStopsTheRequest(t *testing.T) {
	req := &postman.Request{Method: "GET", URL: postman.URL{Raw: "http://127.0.0.1:1"}}
	item := &postman.Item{Name: "broken", Request: req, Events: []postman.Event{prerequest("notDefined();")}}

	resp, _ := NewExecutor().ExecuteContext(context.Background(), req, scopeFor(nil, item), nil)

	if !errors.Is(resp.Error, ErrPreRequestScript) {
		t.Errorf("expected ErrPreRequestScript, got %v", resp.Error)
	}
}

func TestExecute_TestChainSeesResponseAndSendRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/claims", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"c1"}`))
	})
	mux.HandleFunc("/claims/c1", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authKey") != "anything" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"status":"open"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	req := &postman.Request{Method: "POST", URL: postman.URL{Raw: server.URL + "/claims"}}
	collection := &postman.Collection{
		Variables: []postman.Variable{{Key: "base", Value: server.URL}},
		Events:    []postman.Event{testEvent("pm.test('no server error', () => pm.expect(pm.response.code).to.be.below(500));")},
	}
	item := &postman.Item{Name: "create claim", Request: req, Events: []postman.Event{testEvent(
		"const id = pm.response.json().id;",
		"pm.collectionVariables.set('claim_id', id);",
		"pm.sendRequest({ url: `${pm.variables.get('base')}/claims/${id}`, header: { authKey: 'anything' } }, (err, res) => {",
		"  pm.test('claim readable', () => pm.expect(res.json().status).to.eql('open'));",
		"});",
	)}}

	resp, result := NewExecutor().ExecuteContext(context.Background(), req, scopeFor(collection, item), nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected script errors: %v", result.Errors)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("expected collection + callback tests, got %+v", result.Tests)
	}
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("%s: %s", test.Name, test.Error)
		}
	}
	found := false
	for _, v := range collection.Variables {
		found = found || (v.Key == "claim_id" && v.Value == "c1")
	}
	if !found {
		t.Error("expected claim_id written back to the collection")
	}
}

func TestSendRequest_SendsSpecVerbatim(t *testing.T) {
	var gotPath, gotContentType, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	resp, err := NewExecutor().SendRequest(script.SendRequestSpec{Method: "PUT", URL: server.URL + "/{{notResolved}}", Body: "{}"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}
	if gotPath != "/{{notResolved}}" || gotBody != "{}" || gotContentType != defaultContentType {
		t.Errorf("server got path %q body %q content type %q", gotPath, gotBody, gotContentType)
	}
}

func TestExecute_ResponseTimeExcludesPreRequestScripts(t *testing.T) {
	const preRequestBusyMs = 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()

	req := &postman.Request{Method: "GET", URL: postman.URL{Raw: server.URL}}
	item := &postman.Item{Name: "slow setup", Request: req, Events: []postman.Event{
		prerequest(fmt.Sprintf("const until = Date.now() + %d; while (Date.now() < until) {}", preRequestBusyMs)),
		testEvent(fmt.Sprintf("pm.test('fast response', () => pm.expect(pm.response.responseTime).to.be.below(%d));", preRequestBusyMs)),
	}}

	resp, result := NewExecutor().ExecuteContext(context.Background(), req, scopeFor(&postman.Collection{}, item), nil)

	if len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Errorf("expected responseTime to exclude the pre-request script, got %+v", result.Tests)
	}
	if resp.Duration.Milliseconds() < preRequestBusyMs {
		t.Errorf("expected the step duration to still include the pre-request script, got %v", resp.Duration)
	}
}
