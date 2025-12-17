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
	shortcutCol1, shortcutCol2, col1Width := m.buildShortcutsDisplay(len(contextLines))

	var topSection []string
	topSection = append(topSection, titleLine)
	topSection = append(topSection, "")

	maxLines := len(contextLines)
	if len(shortcutCol1) > maxLines {
		maxLines = len(shortcutCol1)
	}

	const contextColWidth = 45
	const colMargin = 4

	for i := 0; i < maxLines; i++ {
		var contextCol, sc1, sc2 string

		if i < len(contextLines) {
			contextCol = contextLines[i]
		}

		if i < len(shortcutCol1) {
			sc1 = shortcutCol1[i]
		}

		if i < len(shortcutCol2) {
			sc2 = shortcutCol2[i]
		}

		contextVisible := lipgloss.Width(contextCol)
		padding1 := contextColWidth - contextVisible
		if padding1 < 0 {
			padding1 = 1
		}

		line := contextCol + strings.Repeat(" ", padding1) + sc1

		if sc2 != "" {
			sc1Visible := lipgloss.Width(sc1)
			padding2 := col1Width - sc1Visible + colMargin
			if padding2 < colMargin {
				padding2 = colMargin
			}
			line += strings.Repeat(" ", padding2) + sc2
		}

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

	totalRequests := m.countTotalRequests()
	loadedEmoji := "📚"
	lines = append(lines,
		loadedEmoji + " " +
		titleOrangeStyle.Render("Loaded: ") +
		valueWhiteStyle.Render(fmt.Sprintf("%d", totalRequests)))

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

func (m Model) buildShortcutsDisplay(contextHeight int) ([]string, []string, int) {
	shortcutBlueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	descGrayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))

	shortcuts := m.commandRegistry.GetContextualShortcuts(m.mode)

	var formattedShortcuts []string
	maxWidth := 0

	for _, shortcut := range shortcuts {
		parts := strings.SplitN(shortcut, ">", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimPrefix(parts[0], "<")
		desc := strings.TrimSpace(parts[1])

		formatted := shortcutBlueStyle.Render("<"+key+">") + " " + descGrayStyle.Render(desc)
		formattedShortcuts = append(formattedShortcuts, formatted)

		width := lipgloss.Width(formatted)
		if width > maxWidth {
			maxWidth = width
		}
	}

	var col1, col2 []string

	if len(formattedShortcuts) <= contextHeight {
		col1 = formattedShortcuts
	} else {
		splitPoint := contextHeight
		col1 = formattedShortcuts[:splitPoint]
		col2 = formattedShortcuts[splitPoint:]
	}

	return col1, col2, maxWidth
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
		display := ":" + m.commandInput
		if m.commandSuggestion != "" && len(m.commandInput) > 0 {
			fadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			remaining := m.commandSuggestion[len(m.commandInput):]
			display += fadedStyle.Render(remaining)
		}
		return commandBarStyle.Width(m.width).Render(display)
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
