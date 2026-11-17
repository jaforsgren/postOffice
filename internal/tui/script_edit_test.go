package tui

import (
	"os"
	"strings"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

// requestScriptsMsg is what the editor sends back for item's scripts, with
// their files written but not yet edited.
func requestScriptsMsg(t *testing.T, item postman.Item, folderPath []string) VimScriptsEditCompleteMsg {
	t.Helper()
	msg, err := writeScriptBuffers(VimScriptsEditCompleteMsg{
		Buffers:    requestScriptBuffers(item),
		Target:     scriptTargetRequest,
		FolderPath: folderPath,
		ItemName:   item.Name,
	})
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

// writeScriptBuffer stands in for the user editing label's file.
func writeScriptBuffer(t *testing.T, msg VimScriptsEditCompleteMsg, label, content string) {
	t.Helper()
	for _, buffer := range msg.Buffers {
		if buffer.Label == label {
			if err := os.WriteFile(buffer.Path, []byte(content+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("no %s buffer in %+v", label, msg.Buffers)
}

func scriptEvent(listen string, lines ...string) postman.Event {
	return postman.Event{Listen: listen, Script: postman.Script{Type: javascriptType, Exec: lines}}
}

func TestRequestScriptBuffers_OneBufferPerScriptPlusMissingKinds(t *testing.T) {
	item := postman.Item{Name: "r", Request: &postman.Request{}, Events: []postman.Event{
		scriptEvent(testListen, "a"),
		scriptEvent(testListen, "b"),
	}}

	buffers := requestScriptBuffers(item)

	var labels []string
	for _, buffer := range buffers {
		labels = append(labels, buffer.Label)
	}
	if got := strings.Join(labels, ","); got != "test,test-2,pre-request" {
		t.Errorf("expected both tests and an empty pre-request buffer, got %s", got)
	}
	if buffers[1].Content != "b" || buffers[2].EventIndex != noScriptEvent {
		t.Errorf("expected the second test's content and a new pre-request, got %+v", buffers)
	}
}

func TestApplyScriptsEdit_SavesChangedScriptsToTheCollectionFile(t *testing.T) {
	m, collectionPath, _ := requestVariablesModel(t)
	m.collection.Items[0].Events = []postman.Event{scriptEvent(testListen, "old test")}
	msg := requestScriptsMsg(t, m.collection.Items[0], nil)
	writeScriptBuffer(t, msg, testScriptLabel, "line one\nline two")
	writeScriptBuffer(t, msg, preRequestScriptLabel, "pm.variables.set('x', 1)")

	m = m.applyVimScriptsEdit(msg)

	events := m.collection.Items[0].Events
	if len(events) != 2 || strings.Join(events[0].Script.Exec, "|") != "line one|line two" || events[1].Listen != preRequestListen {
		t.Fatalf("expected the test replaced and a pre-request added, got %+v", events)
	}
	if !fileContains(t, collectionPath, "pm.variables.set") || fileContains(t, collectionPath, "old test") {
		t.Errorf("expected the collection file saved (%s)", m.statusMessage)
	}
	if _, err := os.Stat(msg.Dir); !os.IsNotExist(err) {
		t.Errorf("expected the temp directory removed, got %v", err)
	}
}

func TestApplyScriptsEdit_UntouchedScriptsAddNothing(t *testing.T) {
	m, _, _ := requestVariablesModel(t)
	msg := requestScriptsMsg(t, m.collection.Items[0], nil)

	m = m.applyVimScriptsEdit(msg)

	if len(m.collection.Items[0].Events) != 0 || m.statusMessage != noEditorChangesMessage {
		t.Errorf("expected no events and %q, got %+v (%s)", noEditorChangesMessage, m.collection.Items[0].Events, m.statusMessage)
	}
}

func TestApplyScriptsEdit_SavesWorkflowStepScripts(t *testing.T) {
	m, collectionPath, _ := requestVariablesModel(t)
	m.activeWorkflow = &workflow.Workflow{ID: "flow", Steps: []workflow.Step{{ID: "step-1", PreScript: "before"}}}
	m.workflowStepCursor = 0
	msg, err := writeScriptBuffers(VimScriptsEditCompleteMsg{
		Buffers:  []scriptBuffer{{Label: preStepScriptLabel, Content: "before"}, {Label: postStepScriptLabel}},
		Target:   scriptTargetWorkflowStep,
		ItemName: "step-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeScriptBuffer(t, msg, postStepScriptLabel, "after")

	m = m.applyVimScriptsEdit(msg)

	step := m.activeWorkflow.Steps[0]
	if step.PreScript != "before" || step.PostScript != "after" {
		t.Errorf("expected only the post-step script changed, got %+v", step)
	}
	if !fileContains(t, workflow.WorkflowPath(collectionPath, "flow"), "after") {
		t.Errorf("expected the workflow file saved (%s)", m.statusMessage)
	}
}
