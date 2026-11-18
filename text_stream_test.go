package main

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"postOffice/internal/workflow"
)

func streamState(statuses ...string) workflow.ExecutionState {
	var state workflow.ExecutionState
	for i, status := range statuses {
		state.Steps = append(state.Steps, &workflow.StepState{ID: string(rune('a' + i)), Request: "Request " + string(rune('A'+i)), Status: status})
	}
	return state
}

func TestStepStream_WritesEachStepWhenItFinishes(t *testing.T) {
	var out bytes.Buffer
	stream := newStepStream(&out, newTextStyles(&out))

	stream.onChange(streamState(workflow.StatusRunning, workflow.StatusPending))
	if out.Len() != 0 {
		t.Fatalf("expected nothing before a step finishes, got %q", out.String())
	}

	stream.onChange(streamState(workflow.StatusSuccess, workflow.StatusRunning))
	stream.onChange(streamState(workflow.StatusSuccess, workflow.StatusFailed))

	want := "✓ Request A  success\n✗ Request B  failed\n"
	if out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestStepStream_WritesARerunStepAgain(t *testing.T) {
	var out bytes.Buffer
	stream := newStepStream(&out, newTextStyles(&out))

	stream.onChange(streamState(workflow.StatusSuccess))
	stream.onChange(streamState(workflow.StatusRunning))
	stream.onChange(streamState(workflow.StatusFailed))

	if got := strings.Count(out.String(), "Request A"); got != 2 {
		t.Errorf("expected both runs written, got %q", out.String())
	}
}

func TestTextStyles_PlainWithoutATerminal(t *testing.T) {
	var out bytes.Buffer

	if icon := newTextStyles(&out).icon(workflow.StatusFailed); icon != workflow.StatusIcon(workflow.StatusFailed) {
		t.Errorf("expected a plain icon, got %q", icon)
	}
}

func TestTextStyles_ColorsOutcomesOnATerminal(t *testing.T) {
	renderer := lipgloss.NewRenderer(&bytes.Buffer{})
	renderer.SetColorProfile(termenv.ANSI)
	styles := textStylesFor(renderer)

	passed, failed := styles.icon(workflow.StatusSuccess), styles.icon(workflow.StatusFailed)

	if passed == workflow.StatusIcon(workflow.StatusSuccess) || failed == workflow.StatusIcon(workflow.StatusFailed) || passed == failed {
		t.Errorf("expected distinct colored icons, got %q and %q", passed, failed)
	}
}

func TestRun_TextReportStreamsStepsBeforeTheSummary(t *testing.T) {
	f := newFixture(t, apiServer(t))

	_, out, _ := cli(t, "run", f.collectionPath, "-e", f.envPath)

	header, getUser, health, summary := strings.Index(out, "Run "), strings.Index(out, "✓ Users/Get User"), strings.Index(out, "✓ Health"), strings.Index(out, "requests:")
	if header != 0 || getUser < 0 || health < getUser || summary < health || strings.Count(out, "✓ Health") != 1 {
		t.Errorf("expected header, steps once each, then summary:\n%s", out)
	}
}

func folderRunState() workflow.ExecutionState {
	outer := &workflow.StepState{ID: "outer", Folder: "outer", Status: workflow.StatusSuccess, Iteration: 1}
	inner := &workflow.StepState{ID: "outer/a", Request: "outer/a", Groups: []string{"outer"}, Status: workflow.StatusSuccess, Iteration: 1}
	return workflow.ExecutionState{
		Status:    workflow.StatusSuccess,
		Steps:     []*workflow.StepState{outer, inner},
		StepIndex: map[string]*workflow.StepState{outer.ID: outer, inner.ID: inner},
		Logs:      []workflow.LogEntry{{StepID: "outer", Iteration: 1, Message: "inside outer"}},
	}
}

func TestStepStream_WritesRequestsNotFolderSteps(t *testing.T) {
	var out bytes.Buffer
	newStepStream(&out, newTextStyles(&out)).onChange(folderRunState())

	if want := "✓ outer/a  success\n"; out.String() != want {
		t.Errorf("got %q, want %q", out.String(), want)
	}
}

func TestRunReport_CountsRequestsAndLabelsFolderStepLogs(t *testing.T) {
	report := newRunReport(folderRunState(), "Collection", "")

	if report.Summary.Requests != 1 || len(report.Requests) != 1 || report.Requests[0].Step != "outer/a" {
		t.Errorf("requests = %+v, want only outer/a", report.Requests)
	}
	if want := []string{"outer" + folderLogSeparator + "inside outer"}; !slices.Equal(report.Logs, want) {
		t.Errorf("logs = %q, want %q", report.Logs, want)
	}
}
