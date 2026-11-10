package main

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"postOffice/internal/workflow"
)

const (
	passedColor  = lipgloss.Color("10")
	failedColor  = lipgloss.Color("9")
	skippedColor = lipgloss.Color("11")
)

// textStyles color the text report's outcomes. Colors only reach a terminal:
// the renderer detects out's color support (and honors NO_COLOR), so files,
// pipes and CI logs get plain text.
type textStyles struct {
	passed  lipgloss.Style
	failed  lipgloss.Style
	skipped lipgloss.Style
}

func newTextStyles(out io.Writer) textStyles {
	return textStylesFor(lipgloss.NewRenderer(out))
}

func textStylesFor(renderer *lipgloss.Renderer) textStyles {
	return textStyles{
		passed:  renderer.NewStyle().Foreground(passedColor),
		failed:  renderer.NewStyle().Foreground(failedColor),
		skipped: renderer.NewStyle().Foreground(skippedColor),
	}
}

func (s textStyles) icon(status string) string {
	icon := workflow.StatusIcon(status)
	switch status {
	case workflow.StatusSuccess:
		return s.passed.Render(icon)
	case workflow.StatusFailed:
		return s.failed.Render(icon)
	case workflow.StatusSkipped, workflow.StatusStopped:
		return s.skipped.Render(icon)
	default:
		return icon
	}
}

// stepStream writes each step to the text report as soon as it finishes,
// so a long run shows its progress instead of everything at the end.
type stepStream struct {
	out    io.Writer
	styles textStyles
	// statuses is each step run's status in the last state seen; a step is
	// written when it changes to a finished one, again if it is rerun.
	statuses map[stepRun]string
}

func newStepStream(out io.Writer, styles textStyles) *stepStream {
	return &stepStream{out: out, styles: styles, statuses: map[stepRun]string{}}
}

func (s *stepStream) onChange(state workflow.ExecutionState) {
	_, stepLogs := splitLogs(state.Logs)
	for _, step := range state.Steps {
		key := stepRun{step.ID, step.Iteration}
		previous := s.statuses[key]
		s.statuses[key] = step.Status
		if previous == step.Status || !isFinishedStatus(step.Status) {
			continue
		}
		writeRequestText(s.out, s.styles, newRequestReport(step, state.IsMultiIteration(), stepLogs))
	}
}

func isFinishedStatus(status string) bool {
	switch status {
	case workflow.StatusSuccess, workflow.StatusFailed, workflow.StatusSkipped, workflow.StatusStopped:
		return true
	default:
		return false
	}
}
