package tui

import (
	"fmt"
	"postOffice/internal/postman"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderTopBar() string {
	titleOrangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	titleLine := titleOrangeStyle.Render("PostOffice")

	contextLines := m.buildContextInfo()
	shortcutLines := m.buildShortcutsDisplay()

	var topSection []string
	topSection = append(topSection, titleLine)
	topSection = append(topSection, "")

	maxLines := len(contextLines)
	if len(shortcutLines) > maxLines {
		maxLines = len(shortcutLines)
	}

	const contextColWidth = 45

	for i := 0; i < maxLines; i++ {
		var leftCol, rightCol string

		if i < len(contextLines) {
			leftCol = contextLines[i]
		}

		if i < len(shortcutLines) {
			rightCol = shortcutLines[i]
		}

		leftColVisible := lipgloss.Width(leftCol)
		padding := contextColWidth - leftColVisible
		if padding < 0 {
			padding = 1
		}

		line := leftCol + strings.Repeat(" ", padding) + rightCol
		topSection = append(topSection, line)
	}

	content := strings.Join(topSection, "\n")
	return titleStyle.Width(m.width).Render(content)
}

func (m Model) buildContextInfo() []string {
	titleOrangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	valueWhiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	var lines []string

	collectionName := "none"
	collectionEmoji := "📦"
	if m.collection != nil {
		collectionName = m.collection.Info.Name
		if len(collectionName) > 25 {
			collectionName = collectionName[:22] + "..."
		}
	}
	lines = append(lines,
		collectionEmoji + " " +
		titleOrangeStyle.Render("Collection: ") +
		valueWhiteStyle.Render(collectionName))

	totalCollections := len(m.parser.ListCollections())
	collectionsEmoji := "📚"
	lines = append(lines,
		collectionsEmoji + " " +
		titleOrangeStyle.Render("Loaded: ") +
		valueWhiteStyle.Render(fmt.Sprintf("%d", totalCollections)))

	envName := "none"
	envEmoji := "🌍"
	totalEnvs := len(m.parser.ListEnvironments())
	if m.environment != nil {
		envName = m.environment.Name
		if len(envName) > 25 {
			envName = envName[:22] + "..."
		}
	}
	lines = append(lines,
		envEmoji + " " +
		titleOrangeStyle.Render("Environment: ") +
		valueWhiteStyle.Render(fmt.Sprintf("%s <%d>", envName, totalEnvs)))

	unsavedCount := len(m.modifiedCollections) + len(m.modifiedEnvironments)
	unsavedEmoji := "💾"
	if unsavedCount > 0 {
		unsavedEmoji = "⚠️"
	}
	lines = append(lines,
		unsavedEmoji + " " +
		titleOrangeStyle.Render("Unsaved: ") +
		valueWhiteStyle.Render(fmt.Sprintf("%d", unsavedCount)))

	totalRequests := m.countTotalRequests()
	requestEmoji := "🚀"
	lines = append(lines,
		requestEmoji + " " +
		titleOrangeStyle.Render("Requests: ") +
		valueWhiteStyle.Render(fmt.Sprintf("%d", totalRequests)))

	modeEmoji := "📍"
	lines = append(lines,
		modeEmoji + " " +
		titleOrangeStyle.Render("Mode: ") +
		valueWhiteStyle.Render(m.getModeString()))

	return lines
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

func (m Model) buildShortcutsDisplay() []string {
	shortcutBlueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	descGrayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))

	shortcuts := m.commandRegistry.GetContextualShortcuts(m.mode)

	const shortcutsPerLine = 2
	var lines []string
	var currentLine []string

	for i, shortcut := range shortcuts {
		parts := strings.SplitN(shortcut, ">", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimPrefix(parts[0], "<")
		desc := strings.TrimSpace(parts[1])

		formatted := shortcutBlueStyle.Render("<"+key+">") + " " + descGrayStyle.Render(desc)

		currentLine = append(currentLine, formatted)

		if (i+1)%shortcutsPerLine == 0 || i == len(shortcuts)-1 {
			lines = append(lines, strings.Join(currentLine, "  "))
			currentLine = []string{}
		}
	}

	return lines
}

func (m Model) countTotalRequests() int {
	if m.collection == nil {
		return 0
	}
	return m.countRequestsRecursive(m.collection.Items)
}

func (m Model) countRequestsRecursive(items []postman.Item) int {
	count := 0
	for _, item := range items {
		if item.IsRequest() {
			count++
		} else if item.IsFolder() {
			count += m.countRequestsRecursive(item.Items)
		}
	}
	return count
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
