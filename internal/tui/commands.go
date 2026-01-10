package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"postOffice/internal/postman"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
			Name:        "edit-script",
			Aliases:     []string{"es"},
			Description: "Edit scripts for selected request",
			ShortHelp:   ":es",
			Handler:     handleEditScriptCommand,
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
			Description: "Save changes and quit application",
			ShortHelp:   ":wq",
			Handler:     handleWriteQuitCommand,
			AvailableIn: []ViewMode{ModeEdit, ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
		},
		{
			Name:        "changes",
			Aliases:     []string{"ch"},
			Description: "Show unsaved changes",
			ShortHelp:   ":changes",
			Handler:     handleChangesCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables},
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
		{
			Name:        "logs",
			Aliases:     []string{"log"},
			Description: "Show session logs",
			ShortHelp:   ":logs",
			Handler:     handleLogsCommand,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeEnvironments, ModeVariables, ModeResponse, ModeInfo, ModeJSON},
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
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeResponse, ModeEnvironments, ModeChanges},
		},
		{
			Keys:        []string{"ctrl+e"},
			Description: "Execute request",
			ShortHelp:   "ctrl+e",
			Handler:     handleExecuteKey,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Keys:        []string{"ctrl+r"},
			Description: "View/Resend response",
			ShortHelp:   "ctrl+r",
			Handler:     handleResponseViewKey,
			AvailableIn: []ViewMode{ModeRequests, ModeResponse},
		},
		{
			Keys:        []string{"i"},
			Description: "Info",
			ShortHelp:   "i",
			Handler:     handleInfoKey,
			AvailableIn: []ViewMode{ModeRequests, ModeEnvironments, ModeChanges},
		},
		{
			Keys:        []string{"J"},
			Description: "View JSON",
			ShortHelp:   "J",
			Handler:     handleJSONKey,
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
			Keys:        []string{"s"},
			Description: "Restore session",
			ShortHelp:   "s",
			Handler:     handleRestoreSessionKey,
			AvailableIn: []ViewMode{ModeCollections},
		},
		{
			Keys:        []string{"esc", "left", "backspace", "h"},
			Description: "Close/Back",
			ShortHelp:   "esc",
			Handler:     handleBackKey,
			AvailableIn: []ViewMode{ModeResponse, ModeInfo, ModeJSON, ModeLog, ModeCollections, ModeRequests, ModeEnvironments, ModeVariables, ModeChanges},
		},
		{
			Keys:        []string{"up", "k"},
			Description: "Navigate up",
			ShortHelp:   "j/k",
			Handler:     handleUpKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeInfo, ModeJSON, ModeLog, ModeResponse, ModeEnvironments, ModeVariables, ModeChanges},
		},
		{
			Keys:        []string{"down", "j"},
			Description: "Scroll/Navigate",
			ShortHelp:   "j/k",
			Handler:     handleDownKey,
			AvailableIn: []ViewMode{ModeCollections, ModeRequests, ModeInfo, ModeJSON, ModeLog, ModeResponse, ModeEnvironments, ModeVariables, ModeChanges},
		},
		{
			Keys:        []string{"d"},
			Description: "Duplicate",
			ShortHelp:   "d",
			Handler:     handleDuplicateKey,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Keys:        []string{"E"},
			Description: "Edit scripts",
			ShortHelp:   "E",
			Handler:     handleEditScriptKey,
			AvailableIn: []ViewMode{ModeRequests},
		},
		{
			Keys:        []string{"d"},
			Description: "Discard selected",
			ShortHelp:   "d",
			Handler:     handleDiscardSelectedKey,
			AvailableIn: []ViewMode{ModeChanges},
		},
		{
			Keys:        []string{"ctrl+d"},
			Description: "Discard all",
			ShortHelp:   "ctrl+d",
			Handler:     handleDiscardAllKey,
			AvailableIn: []ViewMode{ModeChanges},
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
		cwd, err := os.Getwd()
		if err != nil {
			m.statusMessage = fmt.Sprintf("Failed to get working directory: %v", err)
			return m, nil
		}
		m.fileBrowserCwd = cwd
		m = m.enterFileBrowser("load")
	}
	return m, nil
}

func handleLoadEnvCommand(m Model, args []string) (Model, tea.Cmd) {
	if len(args) > 0 {
		path := strings.Join(args, " ")
		m = m.loadEnvironment(path)
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			m.statusMessage = fmt.Sprintf("Failed to get working directory: %v", err)
			return m, nil
		}
		m.fileBrowserCwd = cwd
		m = m.enterFileBrowser("loadenv")
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

		m.infoViewport.Width = m.width - 8
		m.infoViewport.Height = m.height - 8
		lines := m.buildItemInfoLines()
		content := strings.Join(lines, "\n")
		m.infoViewport.SetContent(content)

		m.statusMessage = "Showing item info (q to close)"
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
		m = m.saveAllModifiedRequests()
	}
	return m, nil
}

func handleWriteQuitCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeEdit {
		m = m.saveEdit()
		if !strings.Contains(m.statusMessage, "Failed") && !strings.Contains(m.statusMessage, "Error") {
			return m, tea.Quit
		}
	} else {
		m = m.saveAllModifiedRequests()
		if !strings.Contains(m.statusMessage, "error") {
			return m, tea.Quit
		}
	}
	return m, nil
}

func handleChangesCommand(m Model, args []string) (Model, tea.Cmd) {
	m.previousMode = m.mode
	m.mode = ModeChanges
	m.cursor = 0
	m.items = []string{}

	for itemID := range m.modifiedRequests {
		m.items = append(m.items, itemID)
	}

	if len(m.items) == 0 {
		m.statusMessage = "No unsaved changes"
		m.mode = m.previousMode
	} else {
		m.statusMessage = fmt.Sprintf("%d unsaved change(s) - <d> discard selected, <ctrl+d> discard all, <esc> close", len(m.items))
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
	m.saveSession()
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

func handleLogsCommand(m Model, args []string) (Model, tea.Cmd) {
	m.previousMode = m.mode
	m.mode = ModeLog
	m.scrollOffset = 0
	m.statusMessage = "Showing session logs (j/k to scroll, esc to close)"
	return m, nil
}

func handleQuitKey(m Model) (Model, tea.Cmd) {
	m.saveSession()
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
	if m.mode == ModeChanges {
		if m.cursor < len(m.items) {
			itemID := m.items[m.cursor]
			m = m.navigateToChangedRequest(itemID)
		}
		return m, nil
	}
	return m.handleSelection(), nil
}

func handleExecuteKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.executeRequest(item)
		}
	}
	return m, nil
}

func handleResponseViewKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeResponse {
		if len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
			item := m.currentItems[m.cursor]
			if item.IsRequest() {
				m = m.executeRequest(item)
				lines := m.buildResponseLines()
				content := strings.Join(lines, "\n")
				m.responseViewport.SetContent(content)
			}
		}
	} else if m.mode == ModeRequests {
		if m.lastResponse != nil {
			m.scrollOffset = 0
			m.mode = ModeResponse

			m.responseViewport.Width = m.width - 8
			m.responseViewport.Height = m.height - 8
			lines := m.buildResponseLines()
			content := strings.Join(lines, "\n")
			m.responseViewport.SetContent(content)

			m.statusMessage = "Showing response (ctrl+r to resend, q to close)"
		} else {
			m.statusMessage = "No response to show. Execute a request first with ctrl+e"
		}
	}
	return m, nil
}

func handleInfoKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
		m.currentInfoItem = &m.currentItems[m.cursor]
		m.scrollOffset = 0
		m.previousMode = m.mode
		m.mode = ModeInfo

		m.infoViewport.Width = m.width - 8
		m.infoViewport.Height = m.height - 8
		lines := m.buildItemInfoLines()
		content := strings.Join(lines, "\n")
		m.infoViewport.SetContent(content)

		m.statusMessage = "Showing item info (q to close)"
	} else if m.mode == ModeEnvironments && m.environment != nil {
		m.scrollOffset = 0
		m.envVarCursor = 0
		m.previousMode = m.mode
		m.mode = ModeInfo

		m.infoViewport.Width = m.width - 8
		m.infoViewport.Height = m.height - 8
		lines := m.buildEnvironmentInfoLines()
		content := strings.Join(lines, "\n")
		m.infoViewport.SetContent(content)

		m.statusMessage = "Showing environment info (q to close)"
	} else if m.mode == ModeChanges && m.cursor < len(m.items) {
		itemID := m.items[m.cursor]
		m = m.showChangeDiff(itemID)
	}
	return m, nil
}

func handleJSONKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		jsonData, err := json.MarshalIndent(item, "", "  ")
		if err != nil {
			m.statusMessage = fmt.Sprintf("Failed to generate JSON: %v", err)
			return m, nil
		}
		m.jsonContent = string(jsonData)
		m.scrollOffset = 0
		m.previousMode = m.mode
		m.mode = ModeJSON

		metrics := m.calculateSplitLayout()
		m.jsonViewport.Width = m.width - 8
		m.jsonViewport.Height = metrics.popupHeight - 4
		title := lipgloss.NewStyle().Bold(true).Render("JSON View (press Esc to close)")
		fullContent := title + "\n\n" + m.jsonContent
		m.jsonViewport.SetContent(fullContent)

		m.statusMessage = "Showing JSON view (press Esc to close)"
	} else if m.mode == ModeEnvironments && m.cursor < len(m.items) {
		envName := m.items[m.cursor]
		if env, exists := m.parser.GetEnvironment(envName); exists {
			jsonData, err := json.MarshalIndent(env, "", "  ")
			if err != nil {
				m.statusMessage = fmt.Sprintf("Failed to generate JSON: %v", err)
				return m, nil
			}
			m.jsonContent = string(jsonData)
			m.scrollOffset = 0
			m.previousMode = m.mode
			m.mode = ModeJSON

			metrics := m.calculateSplitLayout()
			m.jsonViewport.Width = m.width - 8
			m.jsonViewport.Height = metrics.popupHeight - 4
			title := lipgloss.NewStyle().Bold(true).Render("JSON View (press Esc to close)")
			fullContent := title + "\n\n" + m.jsonContent
			m.jsonViewport.SetContent(fullContent)

			m.statusMessage = "Showing JSON view (press Esc to close)"
		}
	}
	return m, nil
}

func handleSearchKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeCollections || m.mode == ModeRequests || m.mode == ModeEnvironments {
		m.searchMode = true
		m.searchInput.SetValue("")
		m.searchInput.Focus()
		m.allItems = m.items
		m.allCurrentItems = m.currentItems
		m.statusMessage = "Enter search query (Esc to cancel, Enter to confirm)"
		return m, m.searchInput.Focus()
	}
	return m, nil
}

func handleRestoreSessionKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeCollections {
		m = m.restoreSession()
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
	if m.mode == ModeJSON {
		if m.previousMode != 0 {
			m.mode = m.previousMode
		} else {
			m.mode = ModeRequests
		}
		m.jsonContent = ""
		m.scrollOffset = 0
		m.statusMessage = "Closed JSON view"
		return m, nil
	}
	if m.mode == ModeLog {
		if m.previousMode != 0 {
			m.mode = m.previousMode
		} else {
			m.mode = ModeRequests
		}
		m.scrollOffset = 0
		m.statusMessage = "Closed logs view"
		return m, nil
	}
	if m.mode == ModeChanges {
		m.mode = m.previousMode
		m.statusMessage = "Closed changes view"
		return m, nil
	}
	if m.searchActive {
		m.searchActive = false
		m.searchInput.SetValue("")
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
	} else if m.mode == ModeInfo || m.mode == ModeResponse || m.mode == ModeJSON {
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
	} else if m.mode == ModeInfo || m.mode == ModeResponse || m.mode == ModeJSON {
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

func handleEditScriptKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeRequests && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.enterScriptSelectionMode(item)
		} else {
			m.statusMessage = "Can only edit scripts for requests, not folders"
		}
	} else {
		m.statusMessage = "No request selected"
	}
	return m, nil
}

func handleDiscardSelectedKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeChanges && m.cursor < len(m.items) {
		itemID := m.items[m.cursor]
		parts := strings.Split(itemID, "/")
		if len(parts) > 0 {
			collectionName := parts[0]

			delete(m.modifiedRequests, itemID)
			delete(m.modifiedItems, itemID)

			if path, exists := m.parser.GetCollectionPath(collectionName); exists {
				m.parser.LoadCollection(path)
				if m.collection != nil && m.collection.Info.Name == collectionName {
					if newCollection, exists := m.parser.GetCollection(collectionName); exists {
						m.collection = newCollection
					}
				}
			}

			m.items = append(m.items[:m.cursor], m.items[m.cursor+1:]...)

			if len(m.items) == 0 {
				m.mode = m.previousMode
				m.modifiedCollections = make(map[string]bool)
				m = m.refreshCurrentView()
				m.statusMessage = "All changes discarded and reloaded from file"
			} else {
				if m.cursor >= len(m.items) {
					m.cursor = len(m.items) - 1
				}
				m.statusMessage = fmt.Sprintf("Discarded changes to %s and reloaded from file", itemID)
			}
		}
	}
	return m, nil
}

func handleDiscardAllKey(m Model) (Model, tea.Cmd) {
	if m.mode == ModeChanges {
		collectionsToReload := make(map[string]bool)
		for collectionName := range m.modifiedCollections {
			collectionsToReload[collectionName] = true
		}

		m.modifiedRequests = make(map[string]*postman.Request)
		m.modifiedItems = make(map[string]bool)
		m.modifiedCollections = make(map[string]bool)

		for collectionName := range collectionsToReload {
			if path, exists := m.parser.GetCollectionPath(collectionName); exists {
				m.parser.LoadCollection(path)
			}
		}

		if m.collection != nil {
			if newCollection, exists := m.parser.GetCollection(m.collection.Info.Name); exists {
				m.collection = newCollection
			}
		}

		m.mode = m.previousMode
		m = m.refreshCurrentView()
		m.statusMessage = "Discarded all changes and reloaded from file"
	}
	return m, nil
}

func handleEditScriptCommand(m Model, args []string) (Model, tea.Cmd) {
	if m.mode == ModeRequests && m.cursor < len(m.currentItems) {
		item := m.currentItems[m.cursor]
		if item.IsRequest() {
			m = m.enterScriptSelectionMode(item)
		} else {
			m.statusMessage = "Can only edit scripts for requests, not folders"
		}
	} else {
		m.statusMessage = "No request selected"
	}
	return m, nil
}
