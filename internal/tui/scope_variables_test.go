package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

func enter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }

func TestCollectionVariables_EditAValueSavesTheCollection(t *testing.T) {
	m, collectionPath, _ := requestVariablesModel(t)
	m, _ = handleCollectionVariablesCommand(m, nil)

	if m.mode != ModeRequestVariables || m.scopeLabel() != "Collection: Claims API" {
		t.Fatalf("expected the collection's variables, got %v %q", m.mode, m.scopeLabel())
	}
	m = pressKeys(t, m, enter(), tea.KeyMsg{Type: tea.KeyCtrlU}, runes("c9"), enter())

	if m.collection.Variables[0].Value != "c9" || !fileContains(t, collectionPath, `"c9"`) {
		t.Errorf("expected claim_id saved as c9 (%s)", m.statusMessage)
	}
}

func TestCollectionVariables_NewVariableThenItsValue(t *testing.T) {
	m, collectionPath, _ := requestVariablesModel(t)
	m, _ = handleCollectionVariablesCommand(m, nil)

	m = pressKeys(t, m, runes(newVariableKey), runes("region"), enter(), runes("eu"), enter())

	rows := m.requestVariableRows()
	if len(rows) != 2 || rows[1].Key != "region" || rows[1].Value != "eu" {
		t.Fatalf("expected region=eu added, got %+v (%s)", rows, m.statusMessage)
	}
	if !fileContains(t, collectionPath, `"region"`) {
		t.Error("expected the collection file to hold region")
	}
}

func TestCollectionVariables_NewVariableRejectsAnExistingName(t *testing.T) {
	m, _, _ := requestVariablesModel(t)
	m, _ = handleCollectionVariablesCommand(m, nil)

	m = pressKeys(t, m, runes(newVariableKey), runes("claim_id"), enter())

	if !m.statusIsError || m.requestVarEdit != requestVarEditNewKey || len(m.requestVariableRows()) != 1 {
		t.Errorf("expected the duplicate refused, got %v %q", m.requestVarEdit, m.statusMessage)
	}
}

func TestCollectionVariables_DeleteVariable(t *testing.T) {
	m, collectionPath, _ := requestVariablesModel(t)
	m, _ = handleCollectionVariablesCommand(m, nil)

	m = pressKeys(t, m, runes(deleteVariableKey))

	if len(m.collection.Variables) != 0 || fileContains(t, collectionPath, `"claim_id"`) {
		t.Errorf("expected claim_id deleted, got %+v", m.collection.Variables)
	}
	if view := m.renderRequestVariables(40); !strings.Contains(view, emptyScopeMessage) {
		t.Errorf("expected the empty hint, got %s", view)
	}
}

func TestFolderVariables_VOnAFolderEditsItsOwnVariables(t *testing.T) {
	m, _, pathB := combinedModel(t)
	m = cursorOn(t, openFolder(t, m, combinedB), ordersFolder)

	m = pressKeys(t, m, runes(requestVariablesKey))
	if !m.requestVarsScope || m.scopeLabel() != "Folder: "+ordersFolder {
		t.Fatalf("expected the Orders folder's variables, got scope=%v %q", m.requestVarsScope, m.scopeLabel())
	}
	m = pressKeys(t, m, runes(newVariableKey), runes("order_id"), enter(), runes("o1"), enter())

	b, _ := m.parser.GetCollection(combinedB)
	if got := b.Items[0].Variables; len(got) != 1 || got[0].Value != "o1" {
		t.Errorf("expected order_id on Orders in Collection B, got %+v (%s)", got, m.statusMessage)
	}
	if !fileContains(t, pathB, "order_id") {
		t.Error("expected Collection B's file saved")
	}
	if resolved := postman.ResolveVariables("{{order_id}}", m.variablesAt([]string{combinedB, ordersFolder})); resolved != "o1" {
		t.Errorf("expected the combined view to resolve the new value, got %q", resolved)
	}
}

func TestFolderVariables_GroupFolderIsItsCollection(t *testing.T) {
	m, _, _ := combinedModel(t)
	m = cursorOn(t, m, combinedA)

	m = pressKeys(t, m, runes(requestVariablesKey))

	if m.scopeLabel() != "Collection: "+combinedA || len(m.requestVariableRows()) != 1 {
		t.Errorf("expected Collection A's own variables, got %q %+v", m.scopeLabel(), m.requestVariableRows())
	}
}
