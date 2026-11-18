package workflow

import (
	"context"
	"encoding/json"
	"postOffice/internal/engine"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"strings"
	"testing"
)

func runnerFor(collection *postman.Collection) *Runner {
	runner := NewRunner(collection, nil, engine.New(http.NewExecutor()))
	runner.ctx = &script.ExecutionContext{CollectionVars: collection.Variables}
	return runner
}

func TestExecuteItem_FailedSendLogsProtocolAndReportsErrorBody(t *testing.T) {
	item := &postman.Item{Name: "Order placed", Request: &postman.Request{Method: "POST", URL: postman.URL{Raw: "asb://orders"}}}
	runner := runnerFor(&postman.Collection{Items: []postman.Item{*item}})

	outcome, err := runner.executeItem(context.Background(), item, item.Name)
	if err != nil {
		t.Fatalf("executeItem() error = %v", err)
	}

	if outcome.raw.Error == nil {
		t.Fatal("expected the send to fail without a connection string")
	}
	var body struct{ Error string }
	if err := json.Unmarshal([]byte(outcome.step.Body), &body); err != nil || body.Error != outcome.raw.Error.Error() {
		t.Errorf("step body = %q, want the send error as JSON", outcome.step.Body)
	}
	if len(runner.state.Logs) != 1 || !strings.HasPrefix(runner.state.Logs[0].Message, "[ASB ERROR]") {
		t.Errorf("logs = %+v, want one [ASB ERROR] entry", runner.state.Logs)
	}
}

func TestExecuteItem_RejectsFolders(t *testing.T) {
	folder := &postman.Item{Name: "Users", Items: []postman.Item{{Name: "List", Request: &postman.Request{Method: "GET"}}}}
	runner := runnerFor(&postman.Collection{Items: []postman.Item{*folder}})

	if _, err := runner.executeItem(context.Background(), folder, folder.Name); err == nil || !strings.Contains(err.Error(), "is not a request") {
		t.Errorf("executeItem(folder) error = %v, want a not-a-request error", err)
	}
}
