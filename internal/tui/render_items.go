package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	itemNameWidth          = 60
	itemNameTruncationTail = "..."
)

func (m Model) renderMainWindow() string {
	metrics := m.calculateLayout()
	return m.renderItemsList(metrics.contentHeight)
}

func (m Model) renderItemsList(availableHeight int) string {
	if len(m.items) == 0 {
		return m.renderEmptyItems(availableHeight)
	}

	content := strings.Join(m.buildItemsListLines(availableHeight, itemNameWidth), "\n")
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - windowWidthInset).
		Render(content)
}

// buildItemsListLines returns the header and the window of item lines around
// the cursor. The window is one line shorter than availableHeight minus the
// header, regardless of the caller's padding.
func (m Model) buildItemsListLines(availableHeight int, nameWidth int) []string {
	var lines []string

	if m.viewingFavorites {
		lines = append(lines, folderStyle.Render("★ Favorites"))
		lines = append(lines, "")
	} else if len(m.breadcrumb) > 0 {
		breadcrumbLine := "/ " + strings.Join(m.breadcrumb, " / ")
		lines = append(lines, folderStyle.Render(breadcrumbLine))
		lines = append(lines, "")
	}

	visibleLines := availableHeight - len(lines) - 1
	startIdx, endIdx := calculateVisibleWindow(m.cursor, len(m.items), visibleLines)

	for i := startIdx; i < endIdx; i++ {
		lines = append(lines, m.formatItemLine(i, nameWidth))
	}

	return lines
}

func (m Model) renderEmptyItems(availableHeight int) string {
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - windowWidthInset).
		Render(m.emptyItemsMessage())
}

func (m Model) emptyItemsMessage() string {
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
	return emptyMsg
}

func (m Model) formatItemLine(index int, nameWidth int) string {
	line := m.items[index]

	modifiedPrefix := ""
	executionInfo := ""

	if m.mode == ModeCollections && m.parser.IsCollectionActive(line) {
		modifiedPrefix = "[x] "
	} else if m.mode == ModeEnvironments && m.parser.IsEnvironmentActive(line) {
		modifiedPrefix = "[x] "
	}

	if m.mode == ModeRequests && index < len(m.currentItems) {
		item := m.currentItems[index]
		if item.IsFolder() && m.hasSavedFolderWorkflow(m.folderPathForIndex(index), item.Name) {
			line += " " + folderWorkflowIndicator
		}
		if item.IsRequest() {
			itemID := m.getRequestIdentifierByPath(m.collection.Info.Name, m.folderPathForIndex(index), item.Name)
			if m.favorites[itemID] {
				modifiedPrefix = "★ "
			}
			if m.isItemModified(itemID) {
				modifiedPrefix += "* "
			}

			if exec, exists := m.requestExecutions[itemID]; exists {
				statusColor := "8"
				if exec.Status == "Sending..." {
					statusColor = "14"
				} else if strings.HasPrefix(exec.Status, "2") {
					statusColor = "10"
				} else if strings.HasPrefix(exec.Status, "3") {
					statusColor = "11"
				} else if strings.HasPrefix(exec.Status, "4") {
					statusColor = "9"
				} else if strings.HasPrefix(exec.Status, "5") {
					statusColor = "1"
				}

				elapsed := time.Since(exec.Timestamp)
				timeStr := formatTimeAgo(elapsed)

				statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor))
				timeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))
				executionInfo = "  " + statusStyle.Render(exec.Status) + " " + timeStyle.Render(timeStr)
			}
		}
	}

	locationInfo := ""
	if m.mode == ModeCollections || m.mode == ModeEnvironments {
		if path, exists := m.itemLocation(line); exists {
			locationInfo = "  " + subtleStyle.Render(path)
		}
	}

	cursor := "  "
	if index == m.cursor {
		cursor = "> "
	}

	// Measured in display cells, not bytes: names hold multi-byte runes (tree markers, "→", emoji
	// such as folderWorkflowIndicator) that would otherwise shift the status column.
	line = ansi.Truncate(line, nameWidth, itemNameTruncationTail)
	line += strings.Repeat(" ", nameWidth-ansi.StringWidth(line))

	fullLine := cursor + modifiedPrefix + line + executionInfo + locationInfo

	if index == m.cursor {
		return selectedItemStyle.Render(fullLine)
	}

	style := m.getItemStyle(m.items[index])
	return style.Render(fullLine)
}

func formatTimeAgo(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	} else if d < time.Hour {
		mins := int(d.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	} else if d < 24*time.Hour {
		hours := int(d.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else {
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	}
}

// itemLocation returns the source file path for a collections/environments list
// entry, so formatItemLine can render it as a location column.
func (m Model) itemLocation(name string) (string, bool) {
	switch m.mode {
	case ModeCollections:
		return m.parser.GetCollectionPath(name)
	case ModeEnvironments:
		return m.parser.GetEnvironmentPath(name)
	default:
		return "", false
	}
}

func (m Model) getItemStyle(itemText string) lipgloss.Style {
	itemText = strings.TrimLeft(itemText, " ")
	if strings.HasPrefix(itemText, "[DIR]") {
		return folderStyle
	}
	if strings.HasPrefix(itemText, "[GET]") || strings.HasPrefix(itemText, "[POST]") ||
		strings.HasPrefix(itemText, "[PUT]") || strings.HasPrefix(itemText, "[DELETE]") ||
		strings.HasPrefix(itemText, "[GRPC]") ||
		strings.HasPrefix(itemText, "[ASB]") || strings.HasPrefix(itemText, "[AMQP]") {
		return requestStyle
	}
	return normalItemStyle
}
