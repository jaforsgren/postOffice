package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/workflow"
)

const ansiEscape = "\x1b["

func finishedRunState() workflow.ExecutionState {
	return workflow.ExecutionState{
		WorkflowName: "Run: SAPI/claims",
		Status:       workflow.StatusFailed,
		Steps: []*workflow.StepState{
			{ID: "1. get claim", Request: "claims/get claim", Status: workflow.StatusSuccess, Progress: 1, Total: 1,
				LastResponse: &internalhttp.Response{Status: "200 OK", Duration: 12 * time.Millisecond}},
			{ID: "2. share claim", Request: "claims/share claim", Status: workflow.StatusSkipped, Progress: 1, Total: 1},
		},
		Logs:  []string{"[claims/get claim] ✓ claim listed", "[claims/share claim] [warn] skipped 'share claim'"},
		Error: "one or more requests failed",
	}
}

func TestRunReportLines_PlainHasNoANSIAndAllSections(t *testing.T) {
	report := strings.Join(runReportLines(finishedRunState(), plainRunStyler), "\n")

	if strings.Contains(report, ansiEscape) {
		t.Errorf("expected no ANSI codes in the editor report, got %q", report)
	}
	for _, want := range []string{
		"Run: SAPI/claims [failed]",
		"✓ 1. get claim",
		"claims/get claim  200 OK  12ms",
		"− 2. share claim",
		"✓ claim listed",
		"[warn] skipped 'share claim'",
		"Error: one or more requests failed",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("expected report to contain %q, got:\n%s", want, report)
		}
	}
}

func TestOpenRunInEditor(t *testing.T) {
	t.Run("nothing to show", func(t *testing.T) {
		m, cmd := createTestModel().openRunInEditor()
		if cmd != nil || !strings.Contains(m.statusMessage, "Nothing to show") {
			t.Errorf("expected a hint and no editor, got %q", m.statusMessage)
		}
	})

	t.Run("launches the editor on a snapshot", func(t *testing.T) {
		m := createTestModel()
		m.workflowState = finishedRunState()
		if _, cmd := m.openRunInEditor(); cmd == nil {
			t.Fatal("expected an editor command")
		}
	})
}

func TestApplyVimRunView_RemovesTheReport(t *testing.T) {
	report := filepath.Join(t.TempDir(), "run.log")
	if err := os.WriteFile(report, []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}

	m := createTestModel().applyVimRunView(VimRunViewCompleteMsg{TempFile: report})

	if _, err := os.Stat(report); !os.IsNotExist(err) {
		t.Error("expected the temp report to be removed")
	}
	if m.statusMessage != "Closed run report" {
		t.Errorf("unexpected status %q", m.statusMessage)
	}
}

func TestStartCollectionRun_EntersRunViewAndBackReturnsToRequests(t *testing.T) {
	m := createTestModel()

	m, cmd := m.startCollectionRun([]string{})

	if cmd == nil {
		t.Fatal("expected the run to start")
	}
	if m.mode != ModeWorkflowRun || !m.isCollectionRun || m.workflowChan == nil {
		t.Fatalf("expected the collection run view, got mode %v run=%v", m.mode, m.isCollectionRun)
	}
	if !strings.HasPrefix(m.workflowState.WorkflowName, "Run: ") {
		t.Errorf("unexpected run name %q", m.workflowState.WorkflowName)
	}

	if again, _ := m.startCollectionRun([]string{}); again.statusMessage != "A run is already in progress" {
		t.Errorf("expected a second run to be refused, got %q", again.statusMessage)
	}

	back, _ := handleBackKey(m)
	if back.mode != ModeRequests {
		t.Errorf("expected esc to return to requests, got mode %v", back.mode)
	}
}

func TestStartCollectionRun_RequiresCollection(t *testing.T) {
	m := createTestModel()
	m.collection = nil

	m, cmd := m.startCollectionRun(nil)

	if cmd != nil || m.statusMessage != "Load a collection first" {
		t.Errorf("expected refusal without a collection, got %q", m.statusMessage)
	}
}

func TestRunCommandIsRegistered(t *testing.T) {
	registry := NewCommandRegistry()
	for _, name := range []string{"run", "runner"} {
		if _, ok := registry.commands[name]; !ok {
			t.Errorf("expected :%s to be registered", name)
		}
	}
}
