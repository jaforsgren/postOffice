package tui

import (
	"postOffice/internal/history"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"strings"
	"testing"
)

// unreachableAMQPURL points at a port nothing listens on, so the publish fails fast.
const unreachableAMQPURL = "amqp://127.0.0.1:1/queue"

func executeItem(t *testing.T, m Model, item postman.Item) (Model, RequestCompleteMsg) {
	t.Helper()
	m, cmd := m.executeRequest(item, m.breadcrumb)
	if cmd == nil {
		t.Fatalf("executeRequest(%s) returned no command: %s", item.Name, m.statusMessage)
	}
	return m, cmd().(RequestCompleteMsg)
}

func TestExecuteRequest_RecordsTheProtocolThatSentIt(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want history.Protocol
	}{
		{"Service Bus", "asb://orders", history.ProtocolServiceBus},
		{"AMQP", unreachableAMQPURL, history.ProtocolAMQP},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := postman.Item{Name: tt.name, Request: &postman.Request{Method: "POST", URL: postman.URL{Raw: tt.url}}}

			m, msg := executeItem(t, sizedTestModel(), item)

			if msg.Protocol != tt.want || msg.Response.Error == nil {
				t.Errorf("msg = %+v, want %s protocol with a send error", msg, tt.want)
			}
			if !strings.HasPrefix(m.statusMessage, "Sending: "+string(tt.want)) {
				t.Errorf("statusMessage = %q, want the %s label", m.statusMessage, tt.want)
			}
		})
	}
}

func TestExecuteRequest_SkippedMessageKeepsProtocolAndEnvironment(t *testing.T) {
	m := sizedTestModel()
	m.environment = &postman.Environment{Name: "dev"}
	item := postman.Item{
		Name:    "Skipped",
		Request: &postman.Request{Method: "POST", URL: postman.URL{Raw: "asb://orders"}},
		Events:  []postman.Event{{Listen: "prerequest", Script: postman.Script{Exec: []string{"pm.execution.skipRequest();"}}}},
	}

	_, msg := executeItem(t, m, item)

	if !msg.Response.Skipped || msg.Response.Status != internalhttp.SkippedStatus {
		t.Errorf("response = %+v, want skipped", msg.Response)
	}
	if msg.Protocol != history.ProtocolServiceBus || msg.EnvironmentName != "dev" {
		t.Errorf("msg = %+v, want the ASB protocol and dev environment", msg)
	}
}

func TestExecuteRequest_SendsUnsavedMessageEdits(t *testing.T) {
	m := sizedTestModel()
	item := postman.Item{Name: "Edited", Request: &postman.Request{Method: "POST", URL: postman.URL{Raw: "asb://orders"}}}
	itemID := m.requestItemID(m.breadcrumb, item.Name)
	edited := *item.Request
	edited.URL.Raw = "asb://edited"
	m.modifiedRequests[itemID] = &edited
	m.modifiedItems[itemID] = true

	m, msg := executeItem(t, m, item)

	if msg.Response.RequestURL != edited.URL.Raw || !msg.IsModified {
		t.Errorf("sent %q (modified %v), want the unsaved %q", msg.Response.RequestURL, msg.IsModified, edited.URL.Raw)
	}
	if !strings.HasPrefix(m.statusMessage, "Sending (unsaved): ASB") {
		t.Errorf("statusMessage = %q, want the unsaved ASB status", m.statusMessage)
	}
}

func TestExecuteRequest_RejectsFolders(t *testing.T) {
	folder := postman.Item{Name: "Folder", Items: []postman.Item{{Name: "Child", Request: &postman.Request{Method: "GET"}}}}

	m, cmd := sizedTestModel().executeRequest(folder, nil)

	if cmd != nil || !strings.HasPrefix(m.statusMessage, "Cannot execute") {
		t.Errorf("executeRequest(folder) = %q with a command %v, want a refusal", m.statusMessage, cmd != nil)
	}
}
