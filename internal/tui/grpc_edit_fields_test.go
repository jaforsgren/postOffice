package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

func grpcEditModel(t *testing.T) Model {
	t.Helper()
	m := createTestModel()
	item := postman.Item{Name: "Check", Request: &postman.Request{Method: "GRPC", URL: postman.URL{Raw: "grpc://localhost:50051/grpc.health.v1.Health/Check"}}}
	m = m.enterEditMode(item, nil)
	if m.editType != EditTypeGRPCRequest {
		t.Fatalf("expected the gRPC edit form, got %v", m.editType)
	}
	m.editFieldCursor = grpcFieldInjectDefaults
	return m
}

func TestGRPCEdit_EnterTogglesInjectDefaults(t *testing.T) {
	m := grpcEditModel(t)

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if !m.editRequest.InjectDefaults || m.editFieldMode {
		t.Fatalf("expected inject defaults on without entering text editing, got %v (field mode %v)", m.editRequest.InjectDefaults, m.editFieldMode)
	}
	if !m.editStateChanged() {
		t.Error("expected the toggle to count as an unsaved change")
	}

	m = pressKeys(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.editRequest.InjectDefaults {
		t.Error("expected a second enter to turn it off again")
	}
}

func TestGRPCEdit_ShowsInjectDefaults(t *testing.T) {
	m := grpcEditModel(t)
	m.editRequest.InjectDefaults = true

	fields := strings.Join(m.buildGRPCEditFields(), "\n")

	if !strings.Contains(fields, "Inject defaults:") || !strings.Contains(fields, onLabel) {
		t.Errorf("expected the inject defaults field shown on, got %s", fields)
	}
	if got := m.getEditFieldCount(); got != len(grpcEditFieldNames) {
		t.Errorf("expected %d fields, got %d", len(grpcEditFieldNames), got)
	}
}
