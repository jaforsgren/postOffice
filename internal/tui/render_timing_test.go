package tui

import (
	"errors"
	"postOffice/internal/http"
	"postOffice/internal/timing"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWaterfallSpan(t *testing.T) {
	tests := []struct {
		name                    string
		offset, duration, total time.Duration
		width                   int
		wantStart, wantLength   int
	}{
		{"full span", 0, 100, 100, 40, 0, 40},
		{"second half", 50, 50, 100, 40, 20, 20},
		{"tiny phase still visible", 10, 1, 1000, 40, 0, 1},
		{"zero duration has no fill", 50, 0, 100, 40, 20, 0},
		{"clamped to width", 90, 50, 100, 40, 36, 4},
		{"zero total", 0, 10, 0, 40, 0, 0},
		{"offset at end stays inside bar", 100, 0, 100, 40, 39, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, length := waterfallSpan(tt.offset, tt.duration, tt.total, tt.width)
			if start != tt.wantStart || length != tt.wantLength {
				t.Errorf("waterfallSpan = (%d, %d), want (%d, %d)", start, length, tt.wantStart, tt.wantLength)
			}
		})
	}
}

func TestWaterfallBar_KeepsFixedWidth(t *testing.T) {
	const width = 20
	bar := waterfallBar(25*time.Millisecond, 50*time.Millisecond, 100*time.Millisecond, width)

	if got := len([]rune(bar)); got != width {
		t.Fatalf("bar width = %d, want %d", got, width)
	}
	want := strings.Repeat(timingBarTrack, 5) + strings.Repeat(timingBarFill, 10) + strings.Repeat(timingBarTrack, 5)
	if bar != want {
		t.Errorf("bar = %q, want %q", bar, want)
	}
}

func TestTimingBarWidthFor(t *testing.T) {
	if got := timingBarWidthFor(200); got != timingMaxBarWidth {
		t.Errorf("wide terminal bar width = %d, want %d", got, timingMaxBarWidth)
	}
	if got := timingBarWidthFor(20); got != timingMinBarWidth {
		t.Errorf("narrow terminal bar width = %d, want %d", got, timingMinBarWidth)
	}
}

func TestBuildTimingLines_ShowsPhaseStatuses(t *testing.T) {
	resp := &http.Response{
		RequestMethod: "GET",
		RequestURL:    "https://example.com",
		Duration:      100 * time.Millisecond,
		Timing: []timing.Phase{
			timing.Unmeasured(http.PhaseDNS, timing.StatusReused),
			timing.Unmeasured(http.PhaseTLS, timing.StatusNotApplicable),
			{Name: http.PhaseTTFB, Status: timing.StatusMeasured, Offset: 0, Duration: 80 * time.Millisecond},
		},
	}

	content := strings.Join(buildTimingLines(resp, timingMaxBarWidth), "\n")

	for _, want := range []string{http.PhaseDNS, timingReusedLabel, timingNotApplicableLabel, "80ms", timingTotalLabel, "100ms", timingBarFill} {
		if !strings.Contains(content, want) {
			t.Errorf("timing popup missing %q:\n%s", want, content)
		}
	}
}

func TestBuildTimingLines_WithoutPhasesShowsTotalOnly(t *testing.T) {
	resp := &http.Response{RequestMethod: "ASB", Duration: 42 * time.Millisecond, Error: errors.New("boom")}

	content := strings.Join(buildTimingLines(resp, timingMaxBarWidth), "\n")

	for _, want := range []string{"No phase breakdown available", timingTotalLabel, "42ms", "not reached"} {
		if !strings.Contains(content, want) {
			t.Errorf("timing popup missing %q:\n%s", want, content)
		}
	}
}

func createResponseModel() Model {
	m := createTestModel()
	m.width = 120
	m.height = 40
	m.previousMode = ModeWorkflowDetail
	m.mode = ModeResponse
	m.lastResponse = &http.Response{RequestMethod: "GET", Status: "200 OK", StatusCode: 200, Duration: time.Millisecond}
	return m
}

func runeKey(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func TestTimingKey_TogglesPopupFromResponseView(t *testing.T) {
	m := createResponseModel()

	m, _ = pressKey(m, runeKey(timingKey))
	if m.mode != ModeTiming {
		t.Fatalf("mode = %v, want ModeTiming", m.mode)
	}
	if !strings.Contains(m.View(), "Request Timing") {
		t.Error("expected timing popup to be rendered")
	}

	m, _ = pressKey(m, runeKey(timingKey))
	if m.mode != ModeResponse {
		t.Errorf("mode = %v, want ModeResponse after toggling off", m.mode)
	}
	if m.previousMode != ModeWorkflowDetail {
		t.Errorf("previousMode = %v, timing popup must not change where the response view returns to", m.previousMode)
	}
}

func TestTimingPopup_EscReturnsToResponse(t *testing.T) {
	m := createResponseModel()
	m.mode = ModeTiming

	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.mode != ModeResponse {
		t.Errorf("mode = %v, want ModeResponse", m.mode)
	}
}

func TestTimingCommand_RequiresResponseView(t *testing.T) {
	m := createTestModel()
	m.lastResponse = &http.Response{}

	m, _ = handleTimingCommand(m, nil)

	if m.mode != ModeRequests {
		t.Errorf("mode = %v, want ModeRequests (unchanged)", m.mode)
	}
}

func TestTimingCommand_OpensPopupFromResponseView(t *testing.T) {
	m := createResponseModel()

	m, _ = m.commandRegistry.ExecuteCommand(m, "timing", nil)

	if m.mode != ModeTiming {
		t.Errorf("mode = %v, want ModeTiming", m.mode)
	}
}

func TestResponseView_ScrollKeysStayWithViewport(t *testing.T) {
	m := createResponseModel()

	m, _ = pressKey(m, runeKey("j"))

	if m.mode != ModeResponse {
		t.Errorf("mode = %v, want ModeResponse", m.mode)
	}
}

func TestResponseView_ColonEntersCommandMode(t *testing.T) {
	m := createResponseModel()

	m, _ = pressKey(m, runeKey(":"))

	if !m.commandMode {
		t.Error("expected ':' to enter command mode from the response view")
	}
}

func TestHelpFromResponse_PreservesResponseReturnMode(t *testing.T) {
	m := createResponseModel()

	m, _ = pressKey(m, runeKey("?"))
	if m.mode != ModeHelp {
		t.Fatalf("mode = %v, want ModeHelp", m.mode)
	}
	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeResponse {
		t.Fatalf("mode = %v, want ModeResponse after closing help", m.mode)
	}
	m, _ = pressKey(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != ModeWorkflowDetail {
		t.Errorf("mode = %v, want ModeWorkflowDetail after closing response", m.mode)
	}
}
