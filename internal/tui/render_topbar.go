package tui

import (
	"fmt"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	topBarSectionRows = 6 // rows under each section heading
	topBarTitleRows   = 2 // the title and the blank line under it
	topBarHeadingRows = 1
	topBarPaddingRows = 2 // titleStyle's vertical padding
	topBarPaddingCols = 2 // titleStyle's horizontal padding

	statusBarPaddingCols = 2 // statusBarStyle's horizontal padding

	// topBarHeight is the rendered height; renderTopBar truncates lines so
	// they never wrap and grow it.
	topBarHeight = topBarTitleRows + topBarHeadingRows + topBarSectionRows + topBarPaddingRows

	topBarNameMax      = 25
	topBarSeparator    = " │ "
	topBarShortcutsGap = 3
	envActiveMarker    = "● "
	envInactiveMarker  = "○ "
)

var (
	topBarHeadingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)
	topBarSeparatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
)

// renderTopBar lays the context, the loaded environments and the shortcuts
// out as headed sections divided by vertical rules.
func (m Model) renderTopBar() string {
	titleOrangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)

	sections := [][]string{
		topBarSection("CONTEXT", m.buildContextInfo()),
		topBarSection("ENVIRONMENTS", m.buildEnvironmentsSection()),
	}
	used := topBarPaddingCols
	for _, section := range sections {
		used += columnWidth(section) + lipgloss.Width(topBarSeparator)
	}
	sections = append(sections, topBarSection("SHORTCUTS", m.buildShortcutsDisplay(m.width-used)))

	lines := strings.Split(titleOrangeStyle.Render("PostOffice")+"\n\n"+joinTopBarSections(sections), "\n")
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], max(m.width-topBarPaddingCols, 0), "")
	}
	return titleStyle.Width(m.width).Render(strings.Join(lines, "\n"))
}

func topBarSection(heading string, lines []string) []string {
	return append([]string{topBarHeadingStyle.Render(heading)}, lines...)
}

func columnWidth(lines []string) int {
	widest := 0
	for _, line := range lines {
		widest = max(widest, lipgloss.Width(line))
	}
	return widest
}

// joinTopBarSections pads every section to its widest line and puts a
// vertical rule between them on every row.
func joinTopBarSections(sections [][]string) string {
	rows := 0
	for _, section := range sections {
		rows = max(rows, len(section))
	}

	separator := topBarSeparatorStyle.Render(topBarSeparator)
	lines := make([]string, rows)
	for i := range lines {
		var cells []string
		for s, section := range sections {
			cell := ""
			if i < len(section) {
				cell = section[i]
			}
			if s < len(sections)-1 {
				cell += strings.Repeat(" ", columnWidth(section)-lipgloss.Width(cell))
			}
			cells = append(cells, cell)
		}
		lines[i] = strings.TrimRight(strings.Join(cells, separator), " ")
	}
	return strings.Join(lines, "\n")
}

func truncateName(name string) string {
	if len(name) > topBarNameMax {
		return name[:topBarNameMax-3] + "..."
	}
	return name
}

func (m Model) buildContextInfo() []string {
	titleOrangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	valueWhiteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var lines []string

	collectionName := "none"
	if m.collection != nil {
		collectionName = truncateName(m.collection.Info.Name)
	}
	lines = append(lines,
		"📦 "+titleOrangeStyle.Render("Collection: ")+
			valueWhiteStyle.Render(collectionName)+
			dimStyle.Render(fmt.Sprintf(" <%d requests>", m.countTotalRequests())))

	lines = append(lines, m.buildLoadedNamesLine("📁", "Collections", m.parser.ListCollections(), m.parser.IsCollectionActive))

	envName := "none"
	envStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")) // red = no env
	if m.environment != nil {
		envName = truncateName(m.environment.Name)
		envStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // green = active
	}
	lines = append(lines, "🌍 "+titleOrangeStyle.Render("Env: ")+envStyle.Render(envName))

	unsavedCount := len(m.modifiedCollections) + len(m.modifiedEnvironments)
	unsavedEmoji := "💾"
	if unsavedCount > 0 {
		unsavedEmoji = "⚠️"
	}
	lines = append(lines,
		unsavedEmoji+" "+
			titleOrangeStyle.Render("Unsaved: ")+
			valueWhiteStyle.Render(fmt.Sprintf("%d", unsavedCount)))

	lines = append(lines,
		"📍 "+
			titleOrangeStyle.Render("Mode: ")+
			valueWhiteStyle.Render(m.getModeString()))
	response, execution := m.topBarResponse()
	responseEmoji := "📬"
	responseValue := "none"
	if response != nil {
		if response.Error != nil {
			responseEmoji = "❌"
			responseValue = "Error"
		} else {
			statusCode := response.Status
			if strings.HasPrefix(statusCode, "2") {
				responseEmoji = "✅"
			} else if strings.HasPrefix(statusCode, "3") {
				responseEmoji = "↗️"
			} else if strings.HasPrefix(statusCode, "4") {
				responseEmoji = "⚠️"
			} else if strings.HasPrefix(statusCode, "5") {
				responseEmoji = "🔥"
			}
			responseValue = statusCode
		}

		if execution != nil {
			responseValue += fmt.Sprintf(" (%s)", formatTimeAgo(time.Since(execution.Timestamp)))
		}
	}
	lines = append(lines,
		responseEmoji+" "+
			titleOrangeStyle.Render("Response: ")+
			valueWhiteStyle.Render(responseValue))

	return lines
}

// buildEnvironmentsSection lists every loaded environment on its own row,
// marking the active ones, with a "+N more" row when they don't all fit.
func (m Model) buildEnvironmentsSection() []string {
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	names := m.parser.ListEnvironments()
	if len(names) == 0 {
		return []string{dimStyle.Render("none loaded")}
	}

	shown := names
	if len(names) > topBarSectionRows {
		shown = names[:topBarSectionRows-1]
	}
	var lines []string
	for _, name := range shown {
		if m.parser.IsEnvironmentActive(name) {
			lines = append(lines, activeStyle.Render(envActiveMarker+truncateName(name)))
		} else {
			lines = append(lines, inactiveStyle.Render(envInactiveMarker+truncateName(name)))
		}
	}
	if len(shown) < len(names) {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("+%d more", len(names)-len(shown))))
	}
	return lines
}

// buildLoadedNamesLine renders every loaded collection/environment name for
// the topbar, so all of them stay visible (not just the currently active
// one) — active names (per isActive) are highlighted, and the list is
// truncated with a "+N" count if it doesn't fit the topbar column.
func (m Model) buildLoadedNamesLine(icon, label string, names []string, isActive func(string) bool) string {
	titleOrangeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	inactiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	prefix := icon + " " + titleOrangeStyle.Render(label+": ")

	if len(names) == 0 {
		return prefix + dimStyle.Render("none")
	}

	const nameBudget = 40 // visible columns available for the joined name list
	const perNameMax = 18 // longest a single name is shown before truncating

	var shown []string
	used := 0
	for i, name := range names {
		display := name
		if len(display) > perNameMax {
			display = display[:perNameMax-1] + "…"
		}
		if used+len(display) > nameBudget && i > 0 {
			break
		}
		style := inactiveStyle
		if isActive(name) {
			style = activeStyle
		}
		shown = append(shown, style.Render(display))
		used += len(display) + 2 // account for the ", " separator
	}

	line := prefix + strings.Join(shown, ", ")
	if len(shown) < len(names) {
		line += dimStyle.Render(fmt.Sprintf(" +%d", len(names)-len(shown)))
	}
	return line
}

// topBarResponse is the response the top bar reports, with its execution when
// known for the time since: the selected step's in the workflow views, else
// the last one sent.
func (m Model) topBarResponse() (*http.Response, *RequestExecution) {
	if m.mode == ModeWorkflowDetail {
		return m.selectedStepResponse()
	}
	if m.lastResponse == nil {
		return nil, nil
	}
	if execution, exists := m.requestExecutions[m.lastExecutedItemID]; exists && m.lastExecutedItemID != "" {
		return m.lastResponse, execution
	}
	return m.lastResponse, nil
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
	case ModeChanges:
		return "Changes"
	case ModeJSON:
		return "JSON"
	case ModeLog:
		return "Logs"
	case ModeFileBrowser:
		return "File Browser"
	case ModeSavedResponses:
		return "Saved Responses"
	case ModeHelp:
		return "Help"
	case ModeGRPCReflect:
		return "gRPC Reflection"
	case ModeRequestVariables:
		return "Request Variables"
	case ModeWorkflows:
		return "Workflows"
	case ModeWorkflowDetail:
		return "Workflow"
	case ModeWorkflowRun:
		return "Workflow Run"
	case ModeHistory:
		return "Request History"
	case ModeHistoryDiff:
		return "History Diff"
	case ModeTiming:
		return "Timing"
	case ModeDependencyGraph:
		return "Dependency Graph"
	case ModeListeners:
		return "Listeners"
	case ModeListenerMessage:
		return "Listened Message"
	default:
		return ""
	}
}

func (m Model) getPathString() string {
	path := "/"
	if len(m.breadcrumb) > 0 {
		path = "/" + strings.Join(m.breadcrumb, "/")
	}
	if m.searchActive && m.searchInput.Value() != "" {
		searchStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		path += searchStyle.Render(fmt.Sprintf(" [search: %s]", m.searchInput.Value()))
	}
	return path
}

// buildShortcutsDisplay flows the view's shortcuts into as many columns of
// topBarSectionRows as fit in width, dropping the ones that don't fit.
func (m Model) buildShortcutsDisplay(width int) []string {
	shortcutBlueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	descGrayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("246"))

	var formatted []string
	for _, shortcut := range m.contextualShortcuts() {
		parts := strings.SplitN(shortcut, ">", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimPrefix(parts[0], "<")
		formatted = append(formatted, shortcutBlueStyle.Render("<"+key+">")+" "+descGrayStyle.Render(strings.TrimSpace(parts[1])))
	}

	rows := make([]string, min(len(formatted), topBarSectionRows))
	used := 0
	for start := 0; start < len(formatted); start += topBarSectionRows {
		column := formatted[start:min(start+topBarSectionRows, len(formatted))]
		columnW := columnWidth(column)
		gap := 0
		if start > 0 {
			gap = topBarShortcutsGap
		}
		if used+gap+columnW > width {
			break
		}
		for i := range rows {
			if i >= len(column) {
				continue
			}
			if start > 0 {
				rows[i] += strings.Repeat(" ", used+gap-lipgloss.Width(rows[i]))
			}
			rows[i] += column[i]
		}
		used += gap + columnW
	}
	return rows
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
	shortcuts := m.contextualShortcuts()
	return strings.Join(shortcuts, "  ")
}

func (m Model) renderCommandBar() string {
	if m.commandMode {
		display := ":" + m.commandInput.View()
		inputLen := len(m.commandInput.Value())
		if m.commandSuggestion != "" && inputLen > 0 && inputLen <= len(m.commandSuggestion) {
			fadedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
			remaining := m.commandSuggestion[inputLen:]
			display += fadedStyle.Render(remaining)
		}
		return commandBarStyle.Width(m.width).Render(display)
	}
	if m.searchMode {
		return commandBarStyle.Width(m.width).Render("/" + m.searchInput.View())
	}
	if m.pendingConfirmation != nil {
		return commandBarStyle.Width(m.width).Render(m.pendingConfirmation.prompt)
	}
	return commandBarStyle.Width(m.width).Render("")
}

func (m Model) renderStatusBar() string {
	var help string

	switch m.mode {
	case ModeResponse:
		help = runKey + ": resend | t: timing | ya/yb/yr/yy: copy | " + openReportKey + ": open in editor | q: close | j/k: scroll"
	case ModeTiming:
		help = "t/q/esc: close timing"
	case ModeInfo:
		help = "ya/yb/yr/yy: copy | " + openReportKey + ": open in editor | q: close | j/k: scroll"
	case ModeJSON, ModeLog, ModeHistoryDiff:
		help = "q: close | j/k: scroll"
	case ModeListenerMessage:
		help = nextListenedMessageKey + "/" + previousListenedMessageKey + ": next/previous | " + runKey + ": resend | " + saveListenedMessageKey + ": save as request | " + copyListenedMessageKey + ": copy body | " + openReportKey + ": open | q: close | j/k: scroll"
	case ModeListeners:
		help = "enter: inspect | " + runKey + ": resend | /: search | " + listenerFilterKey + ": filter | " + pauseListenersKey + ": pause | " + clearListenedMessagesKey + ": clear | " + cancelRequestKey + ": stop all | esc: back | j/k: navigate"
	case ModeSavedResponses:
		help = "enter: view | d: delete | esc: back | j/k: navigate"
	case ModeCollections, ModeEnvironments:
		help = "q: quit | ↑↓/jk: navigate | space: toggle active | enter: select | backspace/h: back | /: search | :: command"
	case ModeHistory:
		help = "enter: replay | d: diff vs current | esc: back | j/k: navigate"
	case ModeDependencyGraph:
		help = "enter/r/" + runWithPrerequisitesKey + ": run with prerequisites | l: jump to request | esc: back | j/k: navigate"
	default:
		help = "q: quit | ↑↓/jk: navigate | enter: select | backspace/h: back | /: search | :: command"
	}

	if m.unseenListenedMessages > 0 {
		help = watchingStyle.Render(fmt.Sprintf(newMessagesIndicator, m.unseenListenedMessages, listenersViewKey)) + " | " + help
	}

	// Wrapping would make the status bar taller than statusHeight.
	help = ansi.Truncate(help, max(m.width-statusBarPaddingCols, 0), "…")

	if m.statusMessage == "" {
		return statusBarStyle.Width(m.width).Render("\n" + help)
	}

	// Status message on its own line, full width, truncated only if wider than the terminal.
	msg := m.statusMessage
	maxMsgRunes := m.width - 4 // account for padding
	if maxMsgRunes > 0 && len([]rune(msg)) > maxMsgRunes {
		msg = string([]rune(msg)[:maxMsgRunes-1]) + "…"
	}
	if m.statusIsError {
		msg = errorMessageStyle.Render(msg)
	}

	return statusBarStyle.Width(m.width).Render(msg + "\n" + help)
}
