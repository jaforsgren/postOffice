package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const formRequestName = "POST Request"

func withFormBody(m Model) Model {
	for i := range m.collection.Items {
		if m.collection.Items[i].Name == formRequestName {
			m.collection.Items[i].Request.Body = &postman.Body{
				Mode:     postman.BodyModeFormData,
				FormData: []postman.FormParam{{Key: "file", Type: postman.FormParamTypeFile, Src: json.RawMessage(`"/tmp/a.png"`)}},
			}
		}
	}
	return m
}

func findRootItem(m Model, name string) postman.Item {
	for _, item := range m.collection.Items {
		if item.Name == name {
			return item
		}
	}
	return postman.Item{}
}

func findRootRequest(m Model, name string) *postman.Request {
	return findRootItem(m, name).Request
}

func TestApplyVimEditToCollection_KeepsWhatTheEditorDoesNotShow(t *testing.T) {
	m := withFormBody(createTestModel())
	payload := vimEditPayload{Name: formRequestName, Method: "PUT", URL: "https://example.com/api/create", Headers: []postman.Header{{Key: "X", Value: "1"}}}

	m = m.applyVimEditToCollection(VimEditCompleteMsg{OriginalName: formRequestName, ItemID: formRequestName}, payload)

	updated := findRootRequest(m, formRequestName)
	if updated == nil || updated.Method != "PUT" || len(updated.Header) != 1 {
		t.Fatalf("expected the edit to apply, got %+v", updated)
	}
	if updated.Body == nil || updated.Body.Mode != postman.BodyModeFormData || len(updated.Body.FormData) != 1 {
		t.Errorf("expected the form fields to survive the edit, got %+v", updated.Body)
	}
	if len(updated.URL.Path) == 0 {
		t.Error("expected an unchanged URL to keep its parsed parts")
	}
}

func TestApplyPathParamsToReq_KeepsTheBody(t *testing.T) {
	m := withFormBody(createTestModel())
	original := findRootRequest(m, formRequestName)

	modified := applyPathParamsToReq(original, map[string]string{"id": "42"})

	if modified == original {
		t.Fatal("expected a copy")
	}
	if modified.Body == nil || len(modified.Body.FormData) != 1 {
		t.Errorf("expected the form fields to be kept, got %+v", modified.Body)
	}
}

func writeEditorResult(t *testing.T, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "edited.json")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestApplyVimEdit_UnchangedFileLeavesRequestClean(t *testing.T) {
	m := createTestModel()
	original := []byte(`{"name":"POST Request"}`)
	msg := VimEditCompleteMsg{TempFile: writeEditorResult(t, original), OriginalContent: original, OriginalName: formRequestName, ItemID: formRequestName}

	m = m.applyVimEdit(msg)

	if m.modifiedItems[formRequestName] || len(m.modifiedCollections) != 0 {
		t.Errorf("expected nothing marked modified, got items=%v collections=%v", m.modifiedItems, m.modifiedCollections)
	}
	if m.statusMessage != noEditorChangesMessage {
		t.Errorf("expected %q, got %q", noEditorChangesMessage, m.statusMessage)
	}
}

func TestApplyVimBulkEdit_UnchangedFileLeavesCollectionClean(t *testing.T) {
	m := createTestModel()
	original := []byte(`[]`)

	m = m.applyVimBulkEdit(VimBulkEditCompleteMsg{TempFile: writeEditorResult(t, original), OriginalContent: original})

	if len(m.modifiedCollections) != 0 {
		t.Errorf("expected no modified collections, got %v", m.modifiedCollections)
	}
	if len(m.collection.Items) == 0 {
		t.Error("expected the collection items to be left untouched")
	}
}

func TestApplyVimScriptEdit_UnchangedFileKeepsTextArea(t *testing.T) {
	m := createTestModel()
	m.editFieldTextArea.SetValue("pm.test()")
	original := []byte("pm.test()")

	m = m.applyVimScriptEdit(VimScriptEditCompleteMsg{TempFile: writeEditorResult(t, original), OriginalContent: original})

	if m.statusMessage != noEditorChangesMessage {
		t.Errorf("expected %q, got %q", noEditorChangesMessage, m.statusMessage)
	}
}

func TestEditModeEsc_UnchangedRequestIsNotMarkedModified(t *testing.T) {
	m := createTestModel()
	m = m.enterEditMode(findRootItem(m, formRequestName), nil)

	updated, _ := m.handleEditModeKeys(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if len(m.modifiedItems) != 0 || len(m.modifiedCollections) != 0 {
		t.Errorf("expected nothing marked modified, got items=%v collections=%v", m.modifiedItems, m.modifiedCollections)
	}
	if m.mode == ModeEdit {
		t.Error("expected Esc to leave edit mode")
	}
}

func TestEditModeEsc_ChangedRequestIsMarkedModified(t *testing.T) {
	m := createTestModel()
	m = m.enterEditMode(findRootItem(m, formRequestName), nil)
	m.editRequest.Method = "PATCH"

	updated, _ := m.handleEditModeKeys(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)

	if len(m.modifiedCollections) == 0 {
		t.Error("expected the collection to be marked modified")
	}
}
