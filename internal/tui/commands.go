package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type CommandHandler func(Model, []string) (Model, tea.Cmd)

type KeyHandler func(Model) (Model, tea.Cmd)

type Command struct {
	Name        string
	Aliases     []string
	Description string
	ShortHelp   string
	Handler     CommandHandler
	AvailableIn []ViewMode
}

type KeyBinding struct {
	Keys        []string
	Description string
	ShortHelp   string
	Handler     KeyHandler
	AvailableIn []ViewMode
}

type CommandRegistry struct {
	commands    map[string]*Command
	keyBindings []*KeyBinding
}

func NewCommandRegistry() *CommandRegistry {
	registry := &CommandRegistry{
		commands:    make(map[string]*Command),
		keyBindings: []*KeyBinding{},
	}
	registry.registerCommands()
	registry.registerKeyBindings()
	return registry
}

func (cr *CommandRegistry) registerCommands() {
	commands := []*Command{
		{
			Name:        "collections",
			Aliases:     []string{"c", "col"},
			Description: "Switch to collections view",
			ShortHelp:   ":c",
			Handler:     handleCollectionsCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "requests",
			Aliases:     []string{"r", "req", "request"},
			Description: "Switch to requests view",
			ShortHelp:   ":r",
			Handler:     handleRequestsCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "load",
			Aliases:     []string{"l"},
			Description: "Load a collection from file path",
			ShortHelp:   ":l",
			Handler:     handleLoadCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "loadenv",
			Aliases:     []string{"le", "env-load"},
			Description: "Load an environment from file path",
			ShortHelp:   ":le",
			Handler:     handleLoadEnvCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "environments",
			Aliases:     []string{"e", "env", "envs"},
			Description: "Switch to environments view",
			ShortHelp:   ":e",
			Handler:     handleEnvironmentsCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "variables",
			Aliases:     []string{"v", "var", "vars"},
			Description: "Show all variables",
			ShortHelp:   ":v",
			Handler:     handleVariablesCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "info",
			Aliases:     []string{"i"},
			Description: "Show info for selected item",
			ShortHelp:   ":i",
			Handler:     handleInfoCommand,
			AvailableIn: []ViewMode{ModeRequests, ModeCollections},
		},
		{
			Name:        "edit",
			Aliases:     []string{},
			Description: "Edit selected request",
			ShortHelp:   ":edit",
			Handler:     handleEditCommand,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Name:        "write",
			Aliases:     []string{"w"},
			Description: "Save changes",
			ShortHelp:   ":w",
			Handler:     handleWriteCommand,
			AvailableIn: []ViewMode{ModeEdit},
		},
		{
			Name:        "wq",
			Aliases:     []string{},
			Description: "Save changes and exit edit mode",
			ShortHelp:   ":wq",
			Handler:     handleWriteQuitCommand,
			AvailableIn: []ViewMode{ModeEdit},
		},
		{
			Name:        "duplicate",
			Aliases:     []string{"dup"},
			Description: "Duplicate selected request",
			ShortHelp:   ":dup",
			Handler:     handleDuplicateCommand,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Name:        "delete",
			Aliases:     []string{"del"},
			Description: "Delete selected request",
			ShortHelp:   ":del",
			Handler:     handleDeleteCommand,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Name:        "quit",
			Aliases:     []string{"q", "exit"},
			Description: "Quit application",
			ShortHelp:   ":q",
			Handler:     handleQuitCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeResponse, ModeInfo, ModeEnvironments, ModeVariables, ModeEdit},
		},
		{
			Name:        "help",
			Aliases:     []string{"h", "?"},
			Description: "Show help",
			ShortHelp:   ":h",
			Handler:     nil,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeResponse, ModeInfo, ModeEnvironments, ModeVariables, ModeEdit},
		},
		{
			Name:        "debug",
			Aliases:     []string{"d"},
			Description: "Show debug information",
			ShortHelp:   ":d",
			Handler:     handleDebugCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
	}

	for _, cmd := range commands {
		cr.commands[cmd.Name] = cmd
		for _, alias := range cmd.Aliases {
			cr.commands[alias] = cmd
		}
	}
}

func (cr *CommandRegistry) registerKeyBindings() {
	cr.keyBindings = []*KeyBinding{
		{
			Keys:        []string{"q", "ctrl+c"},
			Description: "Quit application",
			ShortHelp:   "q",
			Handler:     handleQuitKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Keys:        []string{"enter"},
			Description: "Select",
			ShortHelp:   "enter",
			Handler:     handleEnterKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeResponse, ModeEnvironments},
		},
		{
			Keys:        []string{"ctrl+r"},
			Description: "Execute",
			ShortHelp:   "ctrl+r",
			Handler:     handleExecuteKey,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Keys:        []string{"i"},
			Description: "Info",
			ShortHelp:   "i",
			Handler:     handleInfoKey,
			AvailableIn: []ViewMode{ModeRequests, ModeEnvironments},
		},
		{
			Keys:        []string{"/"},
			Description: "Search",
			ShortHelp:   "/",
			Handler:     handleSearchKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments},
		},
		{
			Keys:        []string{"esc", "left", "backspace", "h"},
			Description: "Close/Back",
			ShortHelp:   "esc",
			Handler:     handleBackKey,
			AvailableIn: []ViewMode{ModeResponse, ModeInfo, ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Keys:        []string{"up", "k"},
			Description: "Navigate up",
			ShortHelp:   "j/k",
			Handler:     handleUpKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeInfo, ModeResponse, ModeEnvironments, ModeVariables},
		},
		{
			Keys:        []string{"down", "j"},
			Description: "Scroll/Navigate",
			ShortHelp:   "j/k",
			Handler:     handleDownKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeInfo, ModeResponse, ModeEnvironments, ModeVariables},
		},
		{
			Keys:        []string{"d", "ctrl+d"},
			Description: "Duplicate",
			ShortHelp:   "d",
			Handler:     handleDuplicateKey,
			AvailableIn: []ViewMode{ModeRequests},
		},
	}
}

func (cr *CommandRegistry) ExecuteCommand(m Model, cmdName string, args []string) (Model, tea.Cmd) {
	cmdName = strings.TrimSpace(cmdName)
	if cmdName == "" {
		return m, nil
	}

	if cmdName == "help" || cmdName == "h" || cmdName == "?" {
		m.statusMessage = cr.GenerateHelpText()
		return m, nil
	}

	cmd, exists := cr.commands[cmdName]
	if !exists {
		m.statusMessage = fmt.Sprintf("Unknown command: %s (try :help)", cmdName)
		return m, nil
	}

	if cmd.Handler == nil {
		m.statusMessage = fmt.Sprintf("Command not implemented: %s", cmdName)
		return m, nil
	}

	return cmd.Handler(m, args)
}

func (cr *CommandRegistry) HandleKey(m Model, key string) (Model, tea.Cmd, bool) {
	for _, kb := range cr.keyBindings {
		if !isInModes(m.mode, kb.AvailableIn) {
			continue
		}
		for _, k := range kb.Keys {
			if k == key {
				newModel, cmd := kb.Handler(m)
				return newModel, cmd, true
			}
		}
	}
	return m, nil, false
}

func (cr *CommandRegistry) GenerateHelpText() string {
	var parts []string
	seen := make(map[string]bool)

	for name, cmd := range cr.commands {
		if name != cmd.Name {
			continue
		}
		if seen[cmd.Name] {
			continue
		}
		seen[cmd.Name] = true

		nameWithAliases := ":" + cmd.Name
		if len(cmd.Aliases) > 0 {
			nameWithAliases += "/:" + strings.Join(cmd.Aliases, "/:")
		}
		parts = append(parts, nameWithAliases)
	}

	return "Commands: " + strings.Join(parts, " | ") + " | /search"
}

func (cr *CommandRegistry) GetContextualShortcuts(mode ViewMode) []string {
	var shortcuts []string
	seen := make(map[string]bool)

	for _, kb := range cr.keyBindings {
		if !isInModes(mode, kb.AvailableIn) {
			continue
		}
		if kb.Description == "" {
			continue
		}
		if seen[kb.ShortHelp] {
			continue
		}
		seen[kb.ShortHelp] = true
		shortcuts = append(shortcuts, fmt.Sprintf("<%s> %s", kb.ShortHelp, kb.Description))
	}

	return shortcuts
}

func (cr *CommandRegistry) GetAutocompleteSuggestion(input string, mode ViewMode) string {
	input = strings.ToLower(input)
	var matches []string

	for name, cmd := range cr.commands {
		if name != cmd.Name {
			continue
		}
		if !isInModes(mode, cmd.AvailableIn) {
			continue
		}

		if strings.HasPrefix(strings.ToLower(cmd.Name), input) {
			matches = append(matches, cmd.Name)
		}
	}

	if len(matches) == 1 {
		return matches[0]
	}

	return ""
}

func isInModes(mode ViewMode, modes []ViewMode) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}

func handleCollectionsCommand(m Model, args []string) (Model, tea.Cmd) {
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.statusMessage = "Showing collections"
	return m, nil
}

func handleRequestsCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.collection != nil {
		m.mode = ModeRequests
		m = m.loadRequestsList()
		m.statusMessage = "Showing requests"
	} else {
		m.statusMessage = "No collection loaded. Load a collection first."
	}
	return m, nil
}

func handleLoadCommand(m Model, args []string) (Model, tea.Cmd) {
	if len(args) > 0 {
		path := strings.Join(args, " ")
		m = m.loadCollection(path)
	} else {
		m.statusMessage = "Usage: load <path>"
	}
	return m, nil
}

func handleLoadEnvCommand(m Model, args []string) (Model, tea.Cmd) {
	if len(args) > 0 {
		path := strings.Join(args, " ")
		m = m.loadEnvironment(path)
	} else {
		m.statusMessage = "Usage: loadenv <path>"
	}
	return m, nil
}

func handleEnvironmentsCommand(m Model, args []string) (Model, tea.Cmd) {
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	m.statusMessage = "Showing environments"
	return m, nil
}

func handleVariablesCommand(m Model, args []string) (Model, tea.Cmd) {
	m.mode = ModeVariables
	m = m.loadVariablesList()
	m.statusMessage = "Showing all variables"
	return m, nil
}

func handleInfoCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
		m.currentInfoItem = &m.currentItems[m.cursor]
		m.scrollOffset = 0
		m.previousMode = m.mode
		m.mode = ModeInfo
		m.statusMessage = "Showing item info"
	} else if m.mode == ModeCollections {
		m.statusMessage = "Info command is only available in requests mode"
	} else {
		m.statusMessage = "No item selected"
	}
	return m, nil
}

func handleEditCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeRequests && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.enterEditMode(item)
		} else {
			m.statusMessage = "Can only edit requests, not folders"
		}
	} else {
		m.statusMessage = "No editable item selected"
	}
	return m, nil
}

func handleWriteCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeEdit {
		m = m.saveEdit()
	} else {
		m.statusMessage = "Not in edit mode. Use :edit to edit an item first."
	}
	return m, nil
}

func handleWriteQuitCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeEdit {
		m = m.saveEdit()
		if !strings.Contains(m.statusMessage, "Failed") && !strings.Contains(m.statusMessage, "Error") {
			m.mode = m.previousMode
			m.editType = EditTypeNone
			m.editFieldMode = false
			if m.mode == ModeRequests {
				m = m.refreshCurrentView()
			}
		}
	} else {
		m.statusMessage = "Not in edit mode"
	}
	return m, nil
}

func handleDuplicateCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeRequests && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.duplicateRequest(item)
		} else {
			m.statusMessage = "Can only duplicate requests, not folders"
		}
	} else {
		m.statusMessage = "No request selected to duplicate"
	}
	return m, nil
}

func handleDeleteCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeRequests && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.deleteRequest(item)
		} else {
			m.statusMessage = "Can only delete requests, not folders"
		}
	} else {
		m.statusMessage = "No request selected to delete"
	}
	return m, nil
}

func handleQuitCommand(m Model, args []string) (Model, tea.Cmd) {
	return m, tea.Quit
}

func handleDebugCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.collection != nil {
		itemInfo := fmt.Sprintf("Collection: %s, Items count: %d", m.collection.Info.Name, len(m.collection.Items))
		if len(m.collection.Items) > 0 {
			first := m.collection.Items[0]
			itemInfo += fmt.Sprintf(" | First item: name='%s', hasRequest=%v, hasItems=%v, itemsLen=%d",
				first.Name, first.Request != nil, first.Items != nil, len(first.Items))
		}
		m.statusMessage = itemInfo
	} else {
		m.statusMessage = "No collection loaded"
	}
	return m, nil
}

func handleQuitKey(m Model) (Model, tea.Cmd) {
	return m, tea.Quit
}

func handleEnterKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests {
		if len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
			item := m.currentItems[m.cursor]
			if item.IsFolder() {
				m = m.navigateInto(item)
			}
		}
		return m, nil
	}
	return m.handleSelection(), nil
}

func handleExecuteKey(m Model) (Model, tea.Cmd) {
	return m.handleSelection(), nil
}

func handleInfoKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
		m.currentInfoItem = &m.currentItems[m.cursor]
		m.scrollOffset = 0
		m.previousMode = m.mode
		m.mode = ModeInfo
		m.statusMessage = "Showing item info"
	} else if m.mode == ModeEnvironments && m.environment != nil {
		m.scrollOffset = 0
		m.envVarCursor = 0
		m.previousMode = m.mode
		m.mode = ModeInfo
		m.statusMessage = "Showing environment info"
	}
	return m, nil
}

func handleSearchKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeCollections || m.mode == ModeRequests || m.mode == ModeEnvironments {
		m.searchMode = true
		m.searchQuery = ""
		m.allItems = m.items
		m.allCurrentItems = m.currentItems
		m.statusMessage = "Enter search query (Esc to cancel, Enter to confirm)"
	}
	return m, nil
}

func handleBackKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeResponse {
		m.mode = ModeRequests
		m.scrollOffset = 0
		m.statusMessage = "Closed response view"
		return m, nil
	}
	if m.mode == ModeInfo {
		if m.previousMode != 0 {
			m.mode = m.previousMode
		} else {
			m.mode = ModeRequests
		}
		m.currentInfoItem = nil
		m.scrollOffset = 0
		m.statusMessage = "Closed info view"
		return m, nil
	}
	if m.searchActive {
		m.searchActive = false
		m.searchQuery = ""
		m.items = m.allItems
		m.currentItems = m.allCurrentItems
		m.cursor = 0
		m.statusMessage = "Search cleared"
		return m, nil
	}
	if len(m.breadcrumb) > 0 {
		m = m.navigateUp()
	}
	return m, nil
}

func handleUpKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeInfo && m.previousMode == ModeEnvironments && m.environment != nil {
		if m.envVarCursor > 0 {
			m.envVarCursor--
		}
	} else if m.mode == ModeInfo || m.mode == ModeResponse {
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
	} else {
		if m.cursor > 0 {
			m.cursor--
		}
	}
	return m, nil
}

func handleDownKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeInfo && m.previousMode == ModeEnvironments && m.environment != nil {
		if m.envVarCursor < len(m.environment.Values)-1 {
			m.envVarCursor++
		}
	} else if m.mode == ModeInfo || m.mode == ModeResponse {
		m.scrollOffset++
	} else {
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	}
	return m, nil
}

func handleDuplicateKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.duplicateRequest(item)
		} else {
			m.statusMessage = "Can only duplicate requests, not folders"
		}
	} else {
		m.statusMessage = "No request selected to duplicate"
	}
	return m, nil
}
