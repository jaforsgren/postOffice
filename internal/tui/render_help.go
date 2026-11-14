package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	helpHeadingColor = "214"
	helpKeyColor     = "33"
	helpDescColor    = "252"
	helpDimColor     = "240"

	helpDividerWidth     = 50
	helpKeyIndent        = "  "
	helpKeyDescGap       = "  "
	helpMaxKeyWidth      = 34
	helpColumnGap        = 4
	helpShortcutsTitle   = "Shortcuts"
	helpCommandsTitle    = "Commands"
	helpAllContextsTitle = "Keyboard Shortcuts — All Contexts"
	helpCommandPrefix    = ":"
	helpAliasSeparator   = " / "
	// Below this terminal width the commands column is stacked under the
	// shortcuts instead of beside them.
	helpTwoColumnMinWidth = 110
)

var (
	helpHeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(helpHeadingColor))
	helpKeyStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(helpKeyColor))
	helpDescStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(helpDescColor))
	helpDimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color(helpDimColor))
)

func (m Model) renderHelpView() string {
	return m.helpViewport.View()
}

func (m Model) buildHelpContent() string {
	title := helpAllContextsTitle
	if !m.helpShowAll {
		title = fmt.Sprintf("Keyboard Shortcuts — %s", helpModeTitle(m.helpContextMode()))
	}

	shortcuts := m.renderShortcutsColumn()
	commands := renderHelpSection(helpCommandsTitle, m.buildCommandHelpEntries())

	var body string
	if m.width >= helpTwoColumnMinWidth {
		body = lipgloss.JoinHorizontal(lipgloss.Top, shortcuts, strings.Repeat(" ", helpColumnGap), commands)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, shortcuts, "", commands)
	}

	return helpHeadingStyle.Render(title) + "\n\n" + body
}

func (m Model) renderShortcutsColumn() string {
	var sections []string
	for _, group := range m.buildHelpGroups() {
		sections = append(sections, renderHelpSection(group.title, group.entries))
	}
	if len(sections) == 0 {
		return helpHeadingStyle.Render(helpShortcutsTitle)
	}
	return strings.Join(sections, "\n\n")
}

// renderHelpSection renders a heading, a divider and the entries, with the key
// column as wide as the section's longest key (up to helpMaxKeyWidth).
func renderHelpSection(title string, entries []helpEntry) string {
	keyWidth := 0
	for _, entry := range entries {
		keyWidth = max(keyWidth, lipgloss.Width(entry.key))
	}
	keyWidth = min(keyWidth, helpMaxKeyWidth)

	lines := []string{
		helpHeadingStyle.Render(title),
		helpDimStyle.Render(strings.Repeat("─", helpDividerWidth)),
	}
	for _, entry := range entries {
		key := helpKeyStyle.Render(fmt.Sprintf("%-*s", keyWidth, entry.key))
		lines = append(lines, helpKeyIndent+key+helpKeyDescGap+helpDescStyle.Render(entry.desc))
	}
	return strings.Join(lines, "\n")
}

type helpEntry struct {
	key  string
	desc string
}

type helpGroup struct {
	title   string
	entries []helpEntry
}

// helpSections maps the modes that share a help section to its display title,
// in the order sections appear in the "All Contexts" view.
var helpSections = []struct {
	modes []ViewMode
	title string
}{
	{[]ViewMode{ModeCollections}, "Collections"},
	{[]ViewMode{ModeRequests}, "Requests"},
	{[]ViewMode{ModeResponse}, "Response"},
	{[]ViewMode{ModeInfo, ModeJSON, ModeLog}, "Info / JSON / Logs"},
	{[]ViewMode{ModeEnvironments}, "Environments"},
	{[]ViewMode{ModeVariables}, "Variables"},
	{[]ViewMode{ModeEdit}, "Edit"},
	{[]ViewMode{ModeChanges}, "Changes"},
	{[]ViewMode{ModeWorkflows}, "Workflows"},
	{[]ViewMode{ModeWorkflowRun}, "Workflow Run"},
	{[]ViewMode{ModeWorkflowDetail}, "Workflow Detail"},
	{[]ViewMode{ModeSavedResponses}, "Saved Responses"},
	{[]ViewMode{ModeHistory}, "Request History"},
	{[]ViewMode{ModeHistoryDiff}, "History Diff"},
	{[]ViewMode{ModeTiming}, "Timing"},
	{[]ViewMode{ModeDependencyGraph}, "Dependency Graph"},
	{[]ViewMode{ModeListeners}, "Listeners"},
	{[]ViewMode{ModeListenerMessage}, "Listened Message"},
	{[]ViewMode{ModeHelp}, "Help"},
}

// helpModeTitle returns the section title for mode, or a numeric fallback for
// modes without a help section.
func helpModeTitle(mode ViewMode) string {
	for _, section := range helpSections {
		if slices.Contains(section.modes, mode) {
			return section.title
		}
	}
	return fmt.Sprintf("Mode %d", int(mode))
}

// helpContextMode returns the mode the help view should be scoped to when not
// showing all contexts. m.mode is ModeHelp while help is open, so the actual
// context is whatever mode was active right before ':h'/'?' was pressed.
func (m Model) helpContextMode() ViewMode {
	if m.mode == ModeHelp {
		return m.helpReturnMode
	}
	return m.mode
}

func (m Model) buildHelpGroups() []helpGroup {
	contextMode := m.helpContextMode()
	var groups []helpGroup

	for _, section := range helpSections {
		if !m.helpShowAll && !slices.Contains(section.modes, contextMode) {
			continue
		}

		seen := make(map[string]bool)
		var entries []helpEntry
		for _, kb := range m.commandRegistry.keyBindings {
			if kb.Description == "" || seen[kb.ShortHelp] || !availableInAny(section.modes, kb.AvailableIn) {
				continue
			}
			seen[kb.ShortHelp] = true
			entries = append(entries, helpEntry{key: "<" + strings.Join(kb.Keys, "/") + ">", desc: kb.Description})
		}

		if len(entries) > 0 {
			groups = append(groups, helpGroup{title: section.title, entries: entries})
		}
	}

	return groups
}

func availableInAny(modes []ViewMode, available []ViewMode) bool {
	return slices.ContainsFunc(modes, func(mode ViewMode) bool { return isInModes(mode, available) })
}

// buildCommandHelpEntries lists every command once, sorted by name; the
// registry map also holds each alias, which is skipped.
func (m Model) buildCommandHelpEntries() []helpEntry {
	var commands []*Command
	for name, cmd := range m.commandRegistry.commands {
		if name == cmd.Name && cmd.Description != "" {
			commands = append(commands, cmd)
		}
	}
	slices.SortFunc(commands, func(a, b *Command) int { return strings.Compare(a.Name, b.Name) })

	entries := make([]helpEntry, 0, len(commands))
	for _, cmd := range commands {
		names := append([]string{cmd.Name}, cmd.Aliases...)
		for i, name := range names {
			names[i] = helpCommandPrefix + name
		}
		entries = append(entries, helpEntry{key: strings.Join(names, helpAliasSeparator), desc: cmd.Description})
	}
	return entries
}
