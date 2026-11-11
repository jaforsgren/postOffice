package tui

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"postOffice/internal/history"
	"postOffice/internal/postman"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	completionCollectionName = "Completion API"
	completionURLField       = 2
	historyPersonaID         = "persona-from-history"
	sentPersonaName          = "Ann"
	savedResponsePersonaID   = "persona-from-saved-response"
	savedRequestPersonaLabel = "persona-from-saved-request"
)

var completionHistoryTime = time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

// newCompletionTestModel edits the URL of Users/Get User, with claim variables in the
// environment and collection and persona fields in history, saved responses and the collection.
func newCompletionTestModel(t *testing.T) Model {
	t.Helper()
	collection := &postman.Collection{
		Info:      postman.Info{Name: completionCollectionName},
		Variables: []postman.Variable{{Key: "claim_id", Value: "collection-claim"}, {Key: "base_url", Value: "http://localhost"}},
		Items: []postman.Item{{Name: "Users", Items: []postman.Item{
			{Name: "Get User", Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "http://localhost/user"}}},
			{Name: "Create User", Request: &postman.Request{Method: "POST", Body: &postman.Body{Mode: postman.BodyModeRaw, Raw: `{"persona_label": "` + savedRequestPersonaLabel + `"}`}}},
		}}},
	}

	parser := postman.NewParser()
	if err := parser.SaveResponse(completionCollectionName+"/Users/Get User", postman.SavedResponse{
		SavedAt: completionHistoryTime.Add(-time.Hour),
		Body:    `{"persona_id": "` + savedResponsePersonaID + `"}`,
	}); err != nil {
		t.Fatal(err)
	}
	store := history.NewStore("")
	if err := store.Add(history.Entry{ItemName: "Get User", Snapshot: postman.SavedResponse{
		SavedAt:     completionHistoryTime,
		Body:        `{"user": {"persona_id": "` + historyPersonaID + `"}}`,
		RequestBody: `{"persona_name": "` + sentPersonaName + `"}`,
	}}); err != nil {
		t.Fatal(err)
	}

	m := NewModel(parser, store)
	m.collection = collection
	m.environment = &postman.Environment{Name: "Local", Values: []postman.EnvVariable{
		{Key: "claim_id", Value: "env-claim", Enabled: true},
		{Key: "my_claim_type", Value: "travel", Enabled: true},
	}}
	m.mode = ModeRequests
	m = m.loadRequestsList()
	m = m.navigateInto(collection.Items[0])
	m = m.enterEditMode(collection.Items[0].Items[0], m.breadcrumb)

	m.editFieldCursor = completionURLField
	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.editFieldMode {
		t.Fatalf("expected the URL field in edit mode, status: %s", m.statusMessage)
	}
	m.editFieldInput.SetValue("")
	return m
}

func typeText(m Model, text string) Model {
	for _, r := range text {
		m, _ = pressKey(m, runes(string(r)))
	}
	return m
}

func completionInserts(m Model) []string {
	inserts := make([]string, len(m.fieldCompletion.items))
	for i, item := range m.fieldCompletion.items {
		inserts[i] = item.insert
	}
	return inserts
}

func TestFieldCompletion_VariablePrefixMatchesFirstThenContains(t *testing.T) {
	m := typeText(newCompletionTestModel(t), "/x/{{claim")

	want := []string{"{{claim_id}}", "{{my_claim_type}}"}
	if got := completionInserts(m); !reflect.DeepEqual(got, want) {
		t.Errorf("suggestions = %v, want %v", got, want)
	}
}

func TestFieldCompletion_VariableShowsScopeValueAndShadowed(t *testing.T) {
	m := typeText(newCompletionTestModel(t), "{{claim_")

	item := m.fieldCompletion.items[0]
	if item.label != "{{claim_id}} = env-claim" {
		t.Errorf("label = %q", item.label)
	}
	if want := "Environment: Local · shadows Collection: " + completionCollectionName + " = collection-claim"; item.detail != want {
		t.Errorf("detail = %q, want %q", item.detail, want)
	}
}

func TestFieldCompletion_TabInsertsAndCyclesVariables(t *testing.T) {
	m := typeText(newCompletionTestModel(t), "/x/{{claim")

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.editFieldInput.Value(); got != "/x/{{claim_id}}" {
		t.Errorf("after one Tab = %q", got)
	}
	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.editFieldInput.Value(); got != "/x/{{my_claim_type}}" {
		t.Errorf("after two Tabs = %q", got)
	}
	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.editFieldInput.Value(); got != "/x/{{claim_id}}" {
		t.Errorf("expected Tab to wrap around, got %q", got)
	}
}

func TestFieldCompletion_WordSuggestsValuesFromHistorySavedResponsesAndRequests(t *testing.T) {
	m := typeText(newCompletionTestModel(t), "/users/persona_")

	want := []string{historyPersonaID, sentPersonaName, savedResponsePersonaID, savedRequestPersonaLabel}
	if got := completionInserts(m); !reflect.DeepEqual(got, want) {
		t.Errorf("suggestions = %v, want %v", got, want)
	}
	if m.fieldCompletion.kind != completionFieldValues {
		t.Error("expected field value suggestions")
	}
	when := completionHistoryTime.Format(timestampLayout)
	details := []string{
		responseOrigin + " · Get User · " + when,
		sentRequestOrigin + " · Get User · " + when,
		savedResponseOrigin + " · " + completionCollectionName + "/Users/Get User · " + completionHistoryTime.Add(-time.Hour).Format(timestampLayout),
		savedRequestOrigin + " · Users/Create User",
	}
	for i, item := range m.fieldCompletion.items {
		if item.detail != details[i] {
			t.Errorf("detail %d = %q, want %q", i, item.detail, details[i])
		}
	}
}

func TestFieldCompletion_TabReplacesTheTypedWordWithTheValue(t *testing.T) {
	m := typeText(newCompletionTestModel(t), "/users/persona_")

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyTab})

	if got := m.editFieldInput.Value(); got != "/users/"+historyPersonaID {
		t.Errorf("value = %q", got)
	}
}

func TestFieldCompletion_ShortWordAndClosedVariableSuggestNothing(t *testing.T) {
	for _, text := range []string{"/users/pe", "{{claim_id}}", "/users/unknown_field"} {
		m := typeText(newCompletionTestModel(t), text)
		if m.fieldCompletion.active() {
			t.Errorf("%q: expected no suggestions, got %v", text, completionInserts(m))
		}
	}
}

func TestFieldCompletion_ClearedWhenTheFieldIsSaved(t *testing.T) {
	m := typeText(newCompletionTestModel(t), "{{claim")

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.fieldCompletion.active() {
		t.Error("expected the dropdown to close when the field is saved")
	}
}

func TestBuildFieldCompletionDisplay_FollowsTheCursor(t *testing.T) {
	m := newCompletionTestModel(t)
	for i := range maxCompletionRows + 2 {
		m.fieldCompletion.items = append(m.fieldCompletion.items, completionItem{label: "item" + string(rune('a'+i)), detail: "origin"})
	}
	m.fieldCompletion.cursor = maxCompletionRows + 1

	display := ansi.Strip(strings.Join(m.buildFieldCompletionDisplay(), "\n"))

	if !strings.Contains(display, "> item"+string(rune('a'+maxCompletionRows+1))+"  origin") {
		t.Errorf("expected the selected item visible with its origin:\n%s", display)
	}
	if !strings.Contains(display, "... and 2 more") {
		t.Errorf("expected the hidden count:\n%s", display)
	}
}
