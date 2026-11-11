package tui

import (
	"postOffice/internal/postman"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	toggleSplitViewKey = "tab"

	// itemStatusColumnWidth reserves room in the list pane for the cursor,
	// markers and the "<status> <time ago>" column that follow the item name.
	itemStatusColumnWidth = 28
	minSplitItemNameWidth = 16

	splitNotExecutedMessage    = "Not executed yet — press enter to execute"
	splitFolderResponseMessage = "Folders have no response"
)

func (m Model) renderRequestSplitView() string {
	layout := m.calculateRequestSplitLayout()

	rightColumn := lipgloss.JoinVertical(lipgloss.Left,
		m.renderSplitInfoPane(layout),
		m.renderSplitResponsePane(layout),
	)

	return lipgloss.JoinHorizontal(lipgloss.Top, m.renderSplitListPane(layout), rightColumn)
}

// maxPaneScroll stops scrolling once the last line is on screen, so a pane
// never scrolls into blank space.
func maxPaneScroll(lineCount, height int) int {
	return max(lineCount-height, 0)
}

// scrolledContent is the part of lines a pane of height shows from scroll on.
func scrolledContent(lines []string, scroll, height int) string {
	scroll = min(scroll, maxPaneScroll(len(lines), height))
	return strings.Join(lines[scroll:], "\n")
}

func (m Model) renderSplitListPane(layout requestSplitLayout) string {
	content := m.emptyItemsMessage()
	if len(m.items) > 0 {
		lines := m.buildItemsListLines(paneInnerHeight(layout.listHeight), splitItemNameWidth(layout.listWidth))
		content = strings.Join(lines, "\n")
	}

	return renderSplitPane(content, layout.listWidth, layout.listHeight, mainWindowBorderColor)
}

func (m Model) renderSplitInfoPane(layout requestSplitLayout) string {
	content := scrolledContent(m.splitInfoLines(), m.splitPaneScroll[splitPaneInfo], paneInnerHeight(layout.infoHeight))
	return renderSplitPane(content, layout.rightWidth, layout.infoHeight, m.splitPaneBorderColor(splitPaneInfo, popupBorderColor))
}

func (m Model) renderSplitResponsePane(layout requestSplitLayout) string {
	content, borderColor := m.splitResponseContent()
	lines := strings.Split(content, "\n")
	content = scrolledContent(lines, m.splitPaneScroll[splitPaneResponse], paneInnerHeight(layout.responseHeight))
	return renderSplitPane(content, layout.rightWidth, layout.responseHeight, m.splitPaneBorderColor(splitPaneResponse, borderColor))
}

func (m Model) splitInfoLines() []string {
	item, ok := m.selectedItem()
	if !ok {
		return []string{noItemSelectedMessage}
	}
	return m.withInfoItemAtCursor(item).buildItemDetailLines()
}

func (m Model) splitResponseContent() (string, lipgloss.Color) {
	item, ok := m.selectedItem()
	if !ok {
		return noItemSelectedMessage, popupBorderColor
	}
	if !item.IsRequest() {
		return splitFolderResponseMessage, popupBorderColor
	}

	itemID := m.requestItemID(m.folderPathForCursor(), item.Name)
	exec, exists := m.requestExecutions[itemID]
	if !exists {
		return splitNotExecutedMessage, popupBorderColor
	}
	if exec.Response == nil {
		return exec.Status, popupBorderColor
	}

	responseModel := m
	responseModel.lastResponse = exec.Response
	responseModel.lastTestResult = exec.TestResult
	return strings.Join(responseModel.buildResponseLines(), "\n"), responseBorderColor(exec.Response)
}

func (m Model) selectedItem() (postman.Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.currentItems) {
		return postman.Item{}, false
	}
	return m.currentItems[m.cursor], true
}

// withInfoItemAtCursor returns a copy of the model prepared like the Info view
// for item. The breadcrumb is set to the cursor's folder path (as
// executeRequest does) because, in an expanded tree, the item can live deeper
// than m.breadcrumb and its folder variables would otherwise not resolve.
func (m Model) withInfoItemAtCursor(item postman.Item) Model {
	m.currentInfoItem = &item
	m.breadcrumb = m.folderPathForCursor()
	return m
}

func splitItemNameWidth(listWidth int) int {
	return min(max(paneInnerWidth(listWidth)-itemStatusColumnWidth, minSplitItemNameWidth), itemNameWidth)
}

// renderSplitPane truncates content to the pane's inner size first, so long
// lines are cut rather than wrapped and the pane never grows past its layout.
func renderSplitPane(content string, width int, height int, borderColor lipgloss.Color) string {
	inner := lipgloss.NewStyle().
		MaxWidth(paneInnerWidth(width)).
		MaxHeight(paneInnerHeight(height)).
		Render(content)

	return mainWindowStyle.
		BorderForeground(borderColor).
		Width(width).
		Height(height).
		Render(inner)
}
