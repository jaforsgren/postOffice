package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderHelpView() string {
	return m.helpViewport.View()
}

func (m Model) buildHelpContent() string {
	headingStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sectionDivider := dimStyle.Render(strings.Repeat("─", 50))

	var sb strings.Builder

	if m.helpShowAll {
		sb.WriteString(headingStyle.Render("Keyboard Shortcuts — All Contexts"))
	} else {
		sb.WriteString(headingStyle.Render(fmt.Sprintf("Keyboard Shortcuts — %s", m.getModeString())))
	}
	sb.WriteString("\n\n")

	groups := m.buildHelpGroups()
	for _, group := range groups {
		sb.WriteString(headingStyle.Render(group.title))
		sb.WriteString("\n")
		sb.WriteString(sectionDivider)
		sb.WriteString("\n")

		for _, entry := range group.entries {
			key := keyStyle.Render(fmt.Sprintf("%-14s", entry.key))
			desc := descStyle.Render(entry.desc)
			sb.WriteString(fmt.Sprintf("  %s  %s\n", key, desc))
		}
		sb.WriteString("\n")
	}

	// Commands section (always shown)
	sb.WriteString(headingStyle.Render("Commands"))
	sb.WriteString("\n")
	sb.WriteString(sectionDivider)
	sb.WriteString("\n")
	for _, entry := range m.buildCommandHelpEntries() {
		key := keyStyle.Render(fmt.Sprintf("%-14s", entry.key))
		desc := descStyle.Render(entry.desc)
		sb.WriteString(fmt.Sprintf("  %s  %s\n", key, desc))
	}

	return sb.String()
}

type helpEntry struct {
	key  string
	desc string
}

type helpGroup struct {
	title   string
	entries []helpEntry
}

func (m Model) buildHelpGroups() []helpGroup {
	modeNames := map[ViewMode]string{
		ModeCollections:    "Collections",
		ModeRequests:       "Requests",
		ModeResponse:       "Response",
		ModeInfo:           "Info / JSON / Logs",
		ModeEnvironments:   "Environments",
		ModeVariables:      "Variables",
		ModeEdit:           "Edit",
		ModeChanges:        "Changes",
		ModeWorkflows:      "Workflows",
		ModeWorkflowDetail: "Workflow Detail",
		ModeSavedResponses: "Saved Responses",
		ModeHistory:        "History",
	}

	// Order of sections
	orderedModes := []ViewMode{
		ModeCollections,
		ModeRequests,
		ModeResponse,
		ModeInfo,
		ModeEnvironments,
		ModeVariables,
		ModeEdit,
		ModeChanges,
		ModeWorkflows,
		ModeWorkflowDetail,
		ModeSavedResponses,
		ModeHistory,
	}

	// Track seen shorthelp per group to deduplicate
	type dedupeKey struct {
		mode ViewMode
		key  string
	}
	seen := make(map[dedupeKey]bool)

	var groups []helpGroup

	for _, mode := range orderedModes {
		if !m.helpShowAll && mode != m.mode {
			continue
		}

		var entries []helpEntry
		for _, kb := range m.commandRegistry.keyBindings {
			if !isInModes(mode, kb.AvailableIn) {
				continue
			}
			if kb.Description == "" {
				continue
			}
			dk := dedupeKey{mode: mode, key: kb.ShortHelp}
			if seen[dk] {
				continue
			}
			seen[dk] = true

			keyStr := strings.Join(kb.Keys, "/")
			if len(keyStr) > 12 {
				keyStr = keyStr[:12]
			}
			entries = append(entries, helpEntry{key: "<" + keyStr + ">", desc: kb.Description})
		}

		if len(entries) > 0 {
			title := modeNames[mode]
			if title == "" {
				title = fmt.Sprintf("Mode %d", int(mode))
			}
			groups = append(groups, helpGroup{title: title, entries: entries})
		}
	}

	return groups
}

func (m Model) buildCommandHelpEntries() []helpEntry {
	seen := make(map[string]bool)
	var entries []helpEntry

	for name, cmd := range m.commandRegistry.commands {
		if name != cmd.Name || seen[cmd.Name] || cmd.Description == "" {
			continue
		}
		seen[cmd.Name] = true

		key := ":" + cmd.Name
		if len(cmd.Aliases) > 0 {
			key += " / :" + cmd.Aliases[0]
		}
		entries = append(entries, helpEntry{key: key, desc: cmd.Description})
	}

	return entries
}
