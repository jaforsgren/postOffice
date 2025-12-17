package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderTopBar() string {
	var lines []string

	collectionName := "none"
	if m.collection != nil {
		collectionName = m.collection.Info.Name
		if m.modifiedCollections[collectionName] {
			collectionName += " (*)"
		}
	}

	modeStr := m.getModeString()

	path := m.getPathString()

	leftCol := []string{
		fmt.Sprintf("Collection: %s", collectionName),
		fmt.Sprintf("Mode:       %s", modeStr),
		fmt.Sprintf("Path:       %s", path),
		fmt.Sprintf("Items:      %d", len(m.items)),
	}

	rightCol := []string{
		"PostOffice",
	}

	for i := 0; i < len(leftCol); i++ {
		line := leftCol[i]
		if i < len(rightCol) {
			padding := m.width - len(line) - len(rightCol[i]) - 2
			if padding < 0 {
				padding = 0
			}
			line += strings.Repeat(" ", padding) + rightCol[i]
		}
		lines = append(lines, line)
	}

	shortcuts := m.getContextualShortcuts()
	lines = append(lines, "")
	lines = append(lines, shortcuts)

	content := strings.Join(lines, "\n")
	return titleStyle.Width(m.width).Render(content)
}

func (m Model) getModeString() string {
	switch m.mode {
	case ModeCollections:
		return "Collections"
	case ModeRequests:
		return "Requests"
	case ModeResponse:
		return "Response"
	case ModeInfo:
		return "Info"
	case ModeEnvironments:
		return "Environments"
	case ModeVariables:
		return "Variables"
	case ModeEdit:
		return "Edit (*)"
	default:
		return ""
	}
}

func (m Model) getPathString() string {
	path := "/"
	if len(m.breadcrumb) > 0 {
		path = "/" + strings.Join(m.breadcrumb, "/")
	}
	if m.searchActive && m.searchQuery != "" {
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		path += searchStyle.Render(fmt.Sprintf(" [search: %s]", m.searchQuery))
	}
	return path
}

func (m Model) getContextualShortcuts() string {
	shortcuts := m.commandRegistry.GetContextualShortcuts(m.mode)
	return strings.Join(shortcuts, "  ")
}

func (m Model) renderCommandBar() string {
	if m.commandMode {
		return commandBarStyle.Width(m.width).Render(":" + m.commandInput)
	}
	if m.searchMode {
		return commandBarStyle.Width(m.width).Render("/" + m.searchQuery)
	}
	return commandBarStyle.Width(m.width).Render("")
}

func (m Model) renderStatusBar() string {
	help := "q: quit | ↑↓/jk: navigate | enter: select | backspace/h: back | /: search | :: command"
	if m.statusMessage != "" {
		help = m.statusMessage + " | " + help
	}
	return statusBarStyle.Width(m.width).Render(help)
}
