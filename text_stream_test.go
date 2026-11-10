package main

import (
	"bytes"
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
