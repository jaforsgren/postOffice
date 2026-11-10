package tui

import (
	"strings"
	"testing"

	"postOffice/internal/http"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	splitTestWidth  = 120
	splitTestHeight = 40
	folderIndex     = 0
	requestIndex    = 1
)

func createSplitTestModel() Model {
	m := createTestModel()
	m.width = splitTestWidth
	m.height = splitTestHeight
	m.splitViewActive = true
	return m
}

func pressKey(t *testing.T, m Model, msg tea.KeyMsg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestToggleSplitView_TabTogglesInRequestsMode(t *testing.T) {
	m := createTestModel()

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if !m.splitViewActive {
		t.Fatal("expected tab to enable the split view")
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.splitViewActive {
		t.Fatal("expected second tab to disable the split view")
	}
}

func TestToggleSplitView_TabIgnoredInCollectionsMode(t *testing.T) {
	m := createTestModel()
	m.mode = ModeCollections

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.splitViewActive {
		t.Fatal("expected tab to leave the split view off outside requests mode")
	}
}

func TestCalculateRequestSplitLayout_PanesFillMainWindow(t *testing.T) {
	m := createSplitTestModel()
	layout := m.calculateRequestSplitLayout()
	contentHeight := m.calculateLayout().contentHeight

	totalWidth := layout.listWidth + layout.rightWidth + 2*paneBorderSize
	if want := m.width - windowWidthInset + paneBorderSize; totalWidth != want {
		t.Errorf("pane widths add up to %d, want %d", totalWidth, want)
	}
	if layout.listHeight != contentHeight {
		t.Errorf("list height %d, want %d", layout.listHeight, contentHeight)
	}
	rightHeight := layout.infoHeight + layout.responseHeight + paneBorderSize
	if rightHeight != contentHeight {
		t.Errorf("info and response heights add up to %d, want %d", rightHeight, contentHeight)
	}
}

func TestRenderRequestSplitView_FitsMainWindow(t *testing.T) {
	m := createSplitTestModel()
	m.currentItems[requestIndex].Request.URL.Raw = "https://example.com/" + strings.Repeat("segment/", 40)
	m.cursor = requestIndex

	rendered := m.renderRequestSplitView()

	if got, want := lipgloss.Width(rendered), m.width-windowWidthInset+paneBorderSize; got != want {
		t.Errorf("split view width %d, want %d", got, want)
	}
	if got, want := lipgloss.Height(rendered), m.calculateLayout().contentHeight+paneBorderSize; got != want {
		t.Errorf("split view height %d, want %d", got, want)
	}

	single := m
	single.splitViewActive = false
	if got, want := lipgloss.Height(m.View()), lipgloss.Height(single.View()); got != want {
		t.Errorf("full view height %d, want the single-pane height %d", got, want)
	}
	if got := lipgloss.Width(m.View()); got > m.width {
		t.Errorf("full view width %d exceeds terminal width %d", got, m.width)
	}
}

func TestRenderRequestSplitView_InfoPaneFollowsCursor(t *testing.T) {
	m := createSplitTestModel()

	m.cursor = folderIndex
	if rendered := m.renderRequestSplitView(); !strings.Contains(rendered, "Folder") || strings.Contains(rendered, "api/create") {
		t.Errorf("expected folder info for the folder at the cursor, got:\n%s", rendered)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rendered := m.renderRequestSplitView()
	if !strings.Contains(rendered, "https://example.com/api/create") {
		t.Errorf("expected request info for the request at the cursor, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "q: close") {
		t.Error("split info pane must not advertise q: close")
	}
}

func TestRenderRequestSplitView_ResponsePaneTracksExecution(t *testing.T) {
	m := createSplitTestModel()
	m.cursor = requestIndex

	if rendered := m.renderRequestSplitView(); !strings.Contains(rendered, splitNotExecutedMessage) {
		t.Errorf("expected not-executed placeholder, got:\n%s", rendered)
	}

	m = pressKey(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if rendered := m.renderRequestSplitView(); !strings.Contains(rendered, "Sending...") {
		t.Errorf("expected sending status after enter, got:\n%s", rendered)
	}

	item := m.currentItems[requestIndex]
	updated, _ := m.Update(RequestCompleteMsg{
		ItemID:   m.getRequestIdentifierByPath(m.collection.Info.Name, m.folderPathForCursor(), item.Name),
		ItemName: item.Name,
		Response: &http.Response{
			StatusCode: 200,
			Status:     "200 OK",
			Body:       `{"created":true}`,
		},
	})
	m = updated.(Model)

	if m.mode != ModeRequests {
		t.Fatalf("expected to stay in requests mode, got %v", m.mode)
	}
	rendered := m.renderRequestSplitView()
	if !strings.Contains(rendered, "Status: 200 OK") || !strings.Contains(rendered, `"created": true`) {
		t.Errorf("expected response status and body in the response pane, got:\n%s", rendered)
	}
}

func TestSplitItemNameWidth_LeavesRoomForStatusColumn(t *testing.T) {
	tests := []struct {
		name      string
		listWidth int
		want      int
	}{
		{name: "narrow pane keeps minimum", listWidth: 20, want: minSplitItemNameWidth},
		{name: "medium pane reserves status column", listWidth: 60, want: paneInnerWidth(60) - itemStatusColumnWidth},
		{name: "wide pane caps at single-pane width", listWidth: 200, want: itemNameWidth},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := splitItemNameWidth(tt.listWidth); got != tt.want {
				t.Errorf("splitItemNameWidth(%d) = %d, want %d", tt.listWidth, got, tt.want)
			}
		})
	}
}

func TestRenderContent_SplitViewOffRendersMainWindow(t *testing.T) {
	m := createSplitTestModel()
	m.splitViewActive = false

	content := m.renderContent()
	if len(content) != 1 || content[0] != m.renderMainWindow() {
		t.Error("expected the single-pane main window when the split view is off")
	}
}
