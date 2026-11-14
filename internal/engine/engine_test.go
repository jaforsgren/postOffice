package engine

import (
	"context"
	"errors"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/protocol"
	"postOffice/internal/script"
	"testing"
)

const (
	fakeProtocolName history.Protocol = "FAKE"
	fakeLabel                         = "FK"
	sentStatusCode                    = 201
)

// fakeProtocol claims every request and records what it is asked to send.
type fakeProtocol struct {
	request   *postman.Request
	scope     *script.RequestScope
	variables []postman.VariableSource
	// cancelOnSend cancels the send's context, as a user cancelling mid-request does.
	cancelOnSend context.CancelFunc
}

func (p *fakeProtocol) Name() history.Protocol            { return fakeProtocolName }
func (p *fakeProtocol) Claims(item *postman.Item) bool    { return item.IsRequest() }
func (p *fakeProtocol) Label(req *postman.Request) string { return fakeLabel + " " + req.Method }
func (p *fakeProtocol) Execute(
	_ context.Context,
	req *postman.Request,
	scope *script.RequestScope,
	variables []postman.VariableSource,
) (*http.Response, *script.TestResult) {
	p.request, p.scope, p.variables = req, scope, variables
	if p.cancelOnSend != nil {
		p.cancelOnSend()
	}
	return &http.Response{StatusCode: sentStatusCode}, &script.TestResult{}
}

func scriptSender(script.SendRequestSpec) (*script.ResponseData, error) {
	return nil, nil
}

func newFakeEngine() (*Engine, *fakeProtocol) {
	fake := &fakeProtocol{}
	return NewWithRegistry(protocol.NewRegistry(fake), scriptSender), fake
}

func requestItem() *postman.Item {
	return &postman.Item{Name: "Get user", Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "{{host}}/users"}}}
}

func TestPrepare_RejectsFolders(t *testing.T) {
	requestEngine, _ := newFakeEngine()
	folder := &postman.Item{Name: "Users", Items: []postman.Item{*requestItem()}}

	if _, err := requestEngine.Prepare(Call{Item: folder}); !errors.Is(err, protocol.ErrUnclaimedItem) {
		t.Errorf("Prepare(folder) error = %v, want ErrUnclaimedItem", err)
	}
}

func TestExecution_SendsItemRequestWithScopeVariables(t *testing.T) {
	requestEngine, fake := newFakeEngine()
	item := requestItem()
	collection := &postman.Collection{Variables: []postman.Variable{{Key: "host", Value: "https://api.example.com"}}}

	execution, err := requestEngine.Prepare(Call{Item: item, Scope: script.RequestScope{Collection: collection, Item: item}})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	result := execution.Run(context.Background())

	if fake.request != item.Request {
		t.Errorf("sent %+v, want the item's request", fake.request)
	}
	if len(fake.variables) != 1 || fake.variables[0].Key != "host" {
		t.Errorf("variables = %+v, want the scope's collection variables", fake.variables)
	}
	if result.Protocol != fakeProtocolName || result.Response.StatusCode != sentStatusCode || result.TestResult == nil || result.Cancelled {
		t.Errorf("Run() = %+v, want the fake protocol's uncancelled result", result)
	}
}

func TestExecution_SendsOverridingRequest(t *testing.T) {
	requestEngine, fake := newFakeEngine()
	edited := &postman.Request{Method: "POST", URL: postman.URL{Raw: "https://api.example.com/users"}}

	execution, err := requestEngine.Prepare(Call{Item: requestItem(), Request: edited})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	execution.Run(context.Background())

	if fake.request != edited {
		t.Errorf("sent %+v, want the overriding request", fake.request)
	}
	if got := execution.Label(); got != fakeLabel+" POST" {
		t.Errorf("Label() = %q, want the overriding request's label", got)
	}
}

func TestPrepare_DefaultsScriptSender(t *testing.T) {
	requestEngine, fake := newFakeEngine()

	execution, _ := requestEngine.Prepare(Call{Item: requestItem()})
	execution.Run(context.Background())

	if fake.scope == nil || fake.scope.Sender == nil {
		t.Error("expected the engine's pm.sendRequest sender in the scope")
	}
}

func TestPrepare_KeepsCallersScriptSender(t *testing.T) {
	requestEngine, fake := newFakeEngine()
	called := false
	sender := func(script.SendRequestSpec) (*script.ResponseData, error) {
		called = true
		return nil, nil
	}

	execution, _ := requestEngine.Prepare(Call{Item: requestItem(), Scope: script.RequestScope{Sender: sender}})
	execution.Run(context.Background())
	_, _ = fake.scope.Sender(script.SendRequestSpec{})

	if !called {
		t.Error("expected the caller's pm.sendRequest sender to be kept")
	}
}

func TestExecution_ReportsCancelledSend(t *testing.T) {
	requestEngine, fake := newFakeEngine()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.cancelOnSend = cancel

	execution, _ := requestEngine.Prepare(Call{Item: requestItem()})

	if result := execution.Run(ctx); !result.Cancelled {
		t.Errorf("Run() = %+v, want a cancelled result", result)
	}
}

func TestPrepareReplay_SendsRecordedRequestWithoutScripts(t *testing.T) {
	requestEngine, fake := newFakeEngine()
	entry := history.Entry{
		Protocol: fakeProtocolName,
		Snapshot: postman.SavedResponse{RequestMethod: "DELETE", RequestURL: "https://api.example.com/users/1"},
	}

	execution, err := requestEngine.PrepareReplay(entry)
	if err != nil {
		t.Fatalf("PrepareReplay() error = %v", err)
	}
	execution.Run(context.Background())

	if fake.request.Method != "DELETE" || fake.request.URL.Raw != "https://api.example.com/users/1" {
		t.Errorf("sent %+v, want the recorded request", fake.request)
	}
	if fake.scope != nil || fake.variables != nil {
		t.Errorf("scope = %+v, variables = %+v, want neither so no scripts run", fake.scope, fake.variables)
	}
}

func TestPrepareReplay_RejectsUnknownProtocol(t *testing.T) {
	requestEngine, _ := newFakeEngine()

	if _, err := requestEngine.PrepareReplay(history.Entry{Protocol: "SMTP"}); !errors.Is(err, protocol.ErrUnknownProtocol) {
		t.Errorf("PrepareReplay(SMTP) error = %v, want ErrUnknownProtocol", err)
	}
}

func TestLabel(t *testing.T) {
	requestEngine, _ := newFakeEngine()

	if label, ok := requestEngine.Label(requestItem()); !ok || label != fakeLabel+" GET" {
		t.Errorf("Label(request) = %q, %v", label, ok)
	}
	if _, ok := requestEngine.Label(&postman.Item{Name: "Users", Items: []postman.Item{*requestItem()}}); ok {
		t.Error("Label(folder) ok = true, want false")
	}
}
