package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderMainWindow() string {
	metrics := m.calculateLayout()
	return m.renderItemsList(metrics.contentHeight)
}

func (m Model) renderItemsList(availableHeight int) string {
	if len(m.items) == 0 {
		return m.renderEmptyItems(availableHeight)
	}

	var lines []string

	if len(m.breadcrumb) > 0 {
		breadcrumbLine := "/ " + strings.Join(m.breadcrumb, " / ")
		lines = append(lines, folderStyle.Render(breadcrumbLine))
		lines = append(lines, "")
	}

	visibleLines := availableHeight - len(lines) - 1
	startIdx, endIdx := calculateVisibleWindow(m.cursor, len(m.items), visibleLines)

	for i := startIdx; i < endIdx; i++ {
		line := m.formatItemLine(i)
		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderEmptyItems(availableHeight int) string {
	emptyMsg := "No items to display\n\n"
	emptyMsg += "Commands:\n"
	emptyMsg += "  :load <path> or :l <path>     - Load a Postman collection\n"
	emptyMsg += "  :loadenv <path> or :le <path> - Load a Postman environment\n"
	emptyMsg += "  :collections or :c            - List loaded collections\n"
	emptyMsg += "  :environments or :e           - List loaded environments\n"
	emptyMsg += "  :variables or :v              - List all variables with sources\n"
	emptyMsg += "  :requests or :r               - Browse requests in current collection\n"
	emptyMsg += "  / (in lists)                  - Search (recursive for requests)\n"
	emptyMsg += "  :help or :h or :?             - Show this help\n"
	emptyMsg += "  :quit or :q                   - Quit application\n"
	if m.collection != nil {
		emptyMsg += fmt.Sprintf("\n[Collection loaded: %s - %d items]\n", m.collection.Info.Name, len(m.collection.Items))
	}
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(emptyMsg)
}

func (m Model) formatItemLine(index int) string {
	line := m.items[index]

	modifiedPrefix := ""
	if m.mode == ModeRequests && index < len(m.currentItems) {
		item := m.currentItems[index]
		if item.IsRequest() {
			itemID := m.getRequestIdentifier(item)
			if m.isItemModified(itemID) {
				modifiedPrefix = "* "
			}
		}
	}

	if index == m.cursor {
		line = "> " + modifiedPrefix + line
		return selectedItemStyle.Render(line)
	}

	line = "  " + modifiedPrefix + line
	style := m.getItemStyle(m.items[index])
	return style.Render(line)
}

func (m Model) getItemStyle(itemText string) lipgloss.Style {
	if strings.HasPrefix(itemText, "[DIR]") {
		return folderStyle
	}
	if strings.HasPrefix(itemText, "[GET]") || strings.HasPrefix(itemText, "[POST]") ||
		strings.HasPrefix(itemText, "[PUT]") || strings.HasPrefix(itemText, "[DELETE]") {
		return requestStyle
	}
	return normalItemStyle
}
