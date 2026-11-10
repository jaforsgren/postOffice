package tui

import (
	"encoding/json"
	"testing"

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

func findRootRequest(m Model, name string) *postman.Request {
	for _, item := range m.collection.Items {
		if item.Name == name {
			return item.Request
		}
	}
	return nil
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
