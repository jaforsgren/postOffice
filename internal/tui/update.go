package tui

import (
	"context"
	"errors"
	"fmt"
	"postOffice/internal/grpc"
	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/workflow"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.responseViewport.Width = msg.Width - viewportPadding
		m.infoViewport.Width = msg.Width - viewportPadding
		m.jsonViewport.Width = msg.Width - viewportPadding
		m.workflowViewport.Width = msg.Width - viewportPadding
		return m, nil

	case RequestCompleteMsg:
		if msg.Cancelled {
			return m.completeCancelledRequest(msg).continuePrerequisiteRun(msg)
		}
		m = m.forgetInFlightRequest(msg.ItemID)
		sending := m.requestExecutions[msg.ItemID]
		m.lastResponse = msg.Response
		m.lastTestResult = msg.TestResult
		m.lastExecutedItemID = msg.ItemID

		status := "Error"
		if msg.Response.Error == nil {
			status = msg.Response.Status
		}
		m.requestExecutions[msg.ItemID] = &RequestExecution{
			Status:     status,
			Timestamp:  time.Now(),
			Duration:   msg.Response.Duration,
			Response:   msg.Response,
			TestResult: msg.TestResult,
		}

		if msg.Collection != nil && msg.TestResult != nil {
			if err := m.parser.SaveCollection(msg.Collection.Info.Name); err != nil {
				m.statusMessage = fmt.Sprintf("Warning: failed to save collection variables: %v", err)
			}
			m = m.reloadMergedCollection()
		}

		if msg.Environment != nil && msg.TestResult != nil {
			if err := m.saveScriptEnvironment(msg.Environment); err != nil {
				m.statusMessage = fmt.Sprintf("Warning: failed to save environment variables: %v", err)
			}
			m = m.reloadActiveEnvironment()
		}
		m.requestExecutions[msg.ItemID].Inputs = m.completedRequestInputs(sending, msg.Collection)

		if m.mode == ModeVariables {
			m = m.loadVariablesList()
		}

		if msg.Response.Skipped {
			m.statusMessage = fmt.Sprintf("Skipped by pre-request script: %s (see console log)", msg.ItemName)
			logger.Log(fmt.Sprintf("[REQUEST] SKIPPED %s", msg.ItemName))
		} else if msg.Response.Error != nil {
			m.statusMessage = fmt.Sprintf("Request failed: %s - %v", msg.ItemName, msg.Response.Error)
			logger.Log(fmt.Sprintf("[REQUEST] ERROR %s - %v", msg.ItemName, msg.Response.Error))
		} else {
			statusSuffix := ""
			if msg.IsModified {
				statusSuffix = " [unsaved changes]"
			}
			m.statusMessage = fmt.Sprintf("Response: %s - %s (%v)%s", msg.ItemName, msg.Response.Status, msg.Response.Duration, statusSuffix)
			logger.Log(fmt.Sprintf("[REQUEST] %s %s (%v)", msg.Response.Status, msg.ItemName, msg.Response.Duration))
		}

		m = m.recordHistory(msg)

		if msg.Replayed && m.mode == ModeHistory {
			m.previousMode = ModeHistory
			m.mode = ModeResponse
			m.scrollOffset = 0
		}

		if m.mode == ModeResponse {
			m.configureViewport(&m.responseViewport, strings.Join(m.responseViewLines(), "\n"))
		}

		return m.continuePrerequisiteRun(msg)

	case WorkflowMsg:
		m.workflowState = msg.State
		m = m.refreshWorkflowRunViewport()
		if msg.Done {
			runLabel := fmt.Sprintf("Workflow %q", msg.State.WorkflowName)
			if msg.State.Status == workflow.StatusSuccess {
				m.statusMessage = fmt.Sprintf("%s completed — %s", runLabel, msg.State.Summary)
			} else {
				errMsg := msg.State.Error
				if errMsg == "" {
					errMsg = msg.State.Status
				}
				m.statusMessage = fmt.Sprintf("%s %s: %s — %s", runLabel, msg.State.Status, errMsg, msg.State.Summary)
			}
			m.workflowChan = nil
			if m.collection != nil {
				for _, ss := range msg.State.Steps {
					if ss.LastResponse != nil {
						m.requestExecutions[m.workflowStepItemID(ss.Request)] = &RequestExecution{
							Status:    ss.LastResponse.Status,
							Timestamp: time.Now(),
							Duration:  ss.LastResponse.Duration,
							Response:  ss.LastResponse,
						}
					}
				}
			}
			return m, nil
		}
		return m, waitForWorkflow(m.workflowChan)

	case GRPCReflectMsg:
		if msg.Err != nil {
			m.statusMessage = fmt.Sprintf("gRPC reflection failed: %v", msg.Err)
			return m, nil
		}
		m.grpcReflectServices = msg.Services
		m.grpcReflectPhase = GRPCPhaseServices
		m.grpcSelectedService = 0
		m.previousMode = ModeEdit
		m.mode = ModeGRPCReflect
		if len(msg.Services) == 0 {
			m.statusMessage = "No services found via reflection"
			m.mode = ModeEdit
		} else {
			m.statusMessage = fmt.Sprintf("Found %d service(s) — navigate with j/k, Enter to select", len(msg.Services))
		}
		return m, nil

	case VimEditCompleteMsg:
		m = m.applyVimEdit(msg)
		return m, nil

	case VimScriptEditCompleteMsg:
		m = m.applyVimScriptEdit(msg)
		return m, nil

	case VimRequestReportCompleteMsg:
		m = m.applyVimRequestReport(msg)
		return m, nil

	case VimRunViewCompleteMsg:
		m = m.applyVimRunView(msg)
		return m, nil

	case VimEnvEditCompleteMsg:
		m = m.applyVimEnvEdit(msg)
		return m, nil

	case VimWorkflowEditCompleteMsg:
		m = m.applyVimWorkflowEdit(msg)
		return m, nil

	case VimBulkEditCompleteMsg:
		m = m.applyVimBulkEdit(msg)
		return m, nil

	case VimCollectionEditCompleteMsg:
		m = m.applyVimCollectionEdit(msg)
		return m, nil

	case VimDependenciesEditCompleteMsg:
		m = m.applyVimDependenciesEdit(msg)
		return m, nil

	case tea.KeyMsg:
		updated, cmd := m.handleKeyMsg(msg)
		return updated.(Model).recordJump(m), cmd
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if msg.String() == quitKey {
		return handleQuitKey(m)
	}
	if m.pendingFolderRun != nil {
		return m.handleFolderRunConfirmKeys(msg)
	}
	if m.commandMode {
		return m.handleCommandMode(msg)
	}
	if m.searchMode {
		return m.handleSearchMode(msg)
	}
	if m.pendingYank {
		return m.handleYankKey(msg)
	}
	if m.mode == ModeRequests && m.splitViewActive && m.splitFocus != splitPaneNone {
		focused, cmd, handled := m.handleSplitFocusKeys(msg)
		if handled {
			return focused, cmd
		}
		m = focused
	}
	if m.mode == ModeGRPCReflect {
		return m.handleGRPCReflectKeys(msg)
	}
	if m.mode == ModeRequestVariables {
		return m.handleRequestVariablesKeys(msg)
	}
	if m.mode == ModeEdit {
		if m.editFieldMode {
			return m.handleFieldEdit(msg)
		}
		return m.handleEditModeKeys(msg)
	}

	if m.mode == ModeResponse || m.mode == ModeInfo || m.mode == ModeJSON || m.mode == ModeHelp || m.mode == ModeHistoryDiff {
		key := msg.String()
		if key == "esc" || key == "h" || key == "backspace" || key == "q" {
			return m.handleNormalMode(msg)
		}
		if !isViewportScrollKey(msg) {
			return m.handleNormalMode(msg)
		}

		switch m.mode {
		case ModeResponse:
			m.responseViewport, cmd = m.responseViewport.Update(msg)
		case ModeInfo:
			m.infoViewport, cmd = m.infoViewport.Update(msg)
		case ModeJSON:
			m.jsonViewport, cmd = m.jsonViewport.Update(msg)
		case ModeLog:
			m.logsViewport, cmd = m.logsViewport.Update(msg)
		case ModeHelp:
			m.helpViewport, cmd = m.helpViewport.Update(msg)
		case ModeHistoryDiff:
			m.historyDiffViewport, cmd = m.historyDiffViewport.Update(msg)
		}
		return m, cmd
	}

	return m.handleNormalMode(msg)
}

// runKey sends a request, resends the one on screen or runs a workflow, in
// every view, so one key always means "send".
const runKey = "ctrl+e"

// reflectKey (re)loads a gRPC server's services through reflection.
const reflectKey = "ctrl+f"

// quitKey quits from every view, including text inputs and prompts.
const quitKey = "ctrl+c"

const saveResponseKey = "s"

func (m Model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.Type {
	case tea.KeyEsc:
		m.commandMode = false
		m.commandInput.SetValue("")
		m.commandInput.Blur()
		m.commandSuggestion = ""
		m.historyIndex = -1
		return m, nil

	case tea.KeyEnter:
		input := strings.TrimSpace(m.commandInput.Value())
		if input != "" {
			if len(m.commandHistory) == 0 || m.commandHistory[len(m.commandHistory)-1] != input {
				m.commandHistory = append(m.commandHistory, input)
			}
		}
		m, cmd = m.executeCommand()
		m.commandMode = false
		m.commandInput.SetValue("")
		m.commandInput.Blur()
		m.commandSuggestion = ""
		m.historyIndex = -1
		return m, cmd

	case tea.KeyUp:
		if len(m.commandHistory) > 0 {
			if m.historyIndex == -1 {
				m.historyIndex = len(m.commandHistory) - 1
			} else if m.historyIndex > 0 {
				m.historyIndex--
			}
			m.commandInput.SetValue(m.commandHistory[m.historyIndex])
			m.commandSuggestion = ""
		}
		return m, nil

	case tea.KeyDown:
		if m.historyIndex >= 0 {
			if m.historyIndex < len(m.commandHistory)-1 {
				m.historyIndex++
				m.commandInput.SetValue(m.commandHistory[m.historyIndex])
			} else {
				m.historyIndex = -1
				m.commandInput.SetValue("")
			}
			m.commandSuggestion = ""
		}
		return m, nil

	case tea.KeyTab:
		if m.commandSuggestion != "" {
			m.commandInput.SetValue(m.commandSuggestion)
			m.commandInput.CursorEnd()
			m.commandSuggestion = ""
		}
		return m, nil

	default:
		m.commandInput, cmd = m.commandInput.Update(msg)
		m.commandSuggestion = m.getCommandSuggestion()
		return m, cmd
	}
}

func (m Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.Type {
	case tea.KeyEsc:
		m.searchMode = false
		m.searchInput.Blur()
		m = m.clearSearch()
		m.statusMessage = "Search cancelled"
		return m, nil

	case tea.KeyEnter:
		m.searchMode = false
		m.searchInput.Blur()
		results := m.searchResultCount()
		m.searchActive = results > 0 && m.searchInput.Value() != ""
		if results > 0 {
			m.statusMessage = fmt.Sprintf("Found %d results (press Esc to clear search)", results)
		} else {
			m.statusMessage = "No results found"
		}
		return m, nil

	default:
		m.searchInput, cmd = m.searchInput.Update(msg)
		m = m.filterItems()
		return m, cmd
	}
}

func (m Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.mode == ModeFileBrowser {
		return m.handleFileBrowserKeys(msg)
	}

	if key == ":" {
		m.commandMode = true
		m.commandInput.SetValue("")
		return m, m.commandInput.Focus()
	}

	m.statusIsError = false
	newModel, cmd, handled := m.commandRegistry.HandleKey(m, key)
	if handled {
		return newModel, cmd
	}

	return m, nil
}

func (m Model) executeCommand() (Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.commandInput.Value())
	parts := strings.Fields(cmd)

	if len(parts) == 0 {
		return m, nil
	}

	cmdName := parts[0]
	args := []string{}
	if len(parts) > 1 {
		pathStart := strings.Index(cmd, parts[0]) + len(parts[0])
		argStr := strings.TrimSpace(cmd[pathStart:])
		if argStr != "" {
			args = []string{argStr}
		}
	}

	m.statusIsError = false
	return m.commandRegistry.ExecuteCommand(m, cmdName, args)
}

func (m Model) getCommandSuggestion() string {
	input := strings.TrimSpace(m.commandInput.Value())
	if input == "" {
		return ""
	}

	if strings.Contains(input, " ") {
		return getPathSuggestion(input)
	}

	return m.commandRegistry.GetAutocompleteSuggestion(input, m.mode)
}

func (m Model) loadCollectionsList() Model {
	collections := m.parser.ListCollections()
	m.items = collections
	m.cursor = 0
	m.breadcrumb = []string{}
	m.currentItems = []postman.Item{}

	if len(m.items) == 0 {
		m.statusMessage = "No collections loaded yet. Use :load <path> to load a collection"
	}
	return m
}

const viewportPadding = 8

// isViewportScrollKey reports whether msg scrolls a viewport; all other keys belong to the command registry.
// Every viewport in the TUI uses the default key map.
func isViewportScrollKey(msg tea.KeyMsg) bool {
	keyMap := viewport.DefaultKeyMap()
	return key.Matches(msg,
		keyMap.PageDown, keyMap.PageUp,
		keyMap.HalfPageDown, keyMap.HalfPageUp,
		keyMap.Down, keyMap.Up,
		keyMap.Left, keyMap.Right,
	)
}

// traverseToDepth walks items along path and returns the items slice at that depth.
// Returns nil if any path segment is not found as a folder.
func traverseToDepth(items []postman.Item, path []string) []postman.Item {
	current := items
	for _, name := range path {
		found := false
		for _, item := range current {
			if item.IsFolder() && item.Name == name {
				current = item.Items
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

// traverseToDepthPtr walks items along path and returns a pointer to the items slice at
// that depth for mutation. Returns nil if any path segment is not found as a folder.
func traverseToDepthPtr(items *[]postman.Item, path []string) *[]postman.Item {
	current := items
	for _, name := range path {
		found := false
		for i := range *current {
			if (*current)[i].IsFolder() && (*current)[i].Name == name {
				current = &(*current)[i].Items
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return current
}

// configureViewport sizes a viewport to the model's terminal dimensions minus viewportPadding
// and sets its content. vp must be a pointer to a field on the caller's model copy.
func (m Model) configureViewport(vp *viewport.Model, content string) {
	vp.Width = m.width - viewportPadding
	vp.Height = m.height - viewportPadding
	vp.SetContent(content)
}

// treeFolderKey identifies a folder for expand/collapse tracking: the path to
// its parent joined with its own name.
func treeFolderKey(parentPath []string, name string) string {
	return strings.Join(append(slices.Clone(parentPath), name), "/")
}

// buildTreeView flattens items into a display list, recursing into any folder
// whose treeFolderKey is present in expanded. It returns display strings and,
// in parallel, the folder path each flattened entry actually lives in (which
// differs from the caller's own breadcrumb once a nested folder is expanded).
func (m Model) buildTreeView(items []postman.Item, parentPath []string, depth int, expanded map[string]bool) ([]string, []postman.Item, [][]string) {
	var display []string
	var flat []postman.Item
	var paths [][]string

	indent := strings.Repeat("  ", depth)

	for _, item := range items {
		marker := ""
		isExpandedFolder := false
		if item.IsFolder() {
			isExpandedFolder = expanded[treeFolderKey(parentPath, item.Name)]
			if isExpandedFolder {
				marker = "▾ "
			} else {
				marker = "▸ "
			}
		}

		display = append(display, indent+m.itemDisplayPrefix(item)+marker+item.Name)
		flat = append(flat, item)
		paths = append(paths, slices.Clone(parentPath))

		if isExpandedFolder {
			childPath := append(slices.Clone(parentPath), item.Name)
			childDisplay, childFlat, childPaths := m.buildTreeView(item.Items, childPath, depth+1, expanded)
			display = append(display, childDisplay...)
			flat = append(flat, childFlat...)
			paths = append(paths, childPaths...)
		}
	}

	return display, flat, paths
}

// applyTreeItems rebuilds m.items/m.currentItems/m.currentItemPaths for the
// requests view, flattening any folders under itemsFolderPath that are
// expanded in m.expandedFolders.
func (m Model) applyTreeItems(items []postman.Item, itemsFolderPath []string) Model {
	m = m.ensureSavedWorkflowIDs()
	display, flat, paths := m.buildTreeView(items, itemsFolderPath, 0, m.expandedFolders)
	m.items = display
	m.currentItems = flat
	m.currentItemPaths = paths
	return m
}

// folderPathForIndex returns the real folder path of the entry at index in
// the requests view — which is only different from m.breadcrumb when index
// refers to a row revealed by expanding a nested folder with "z".
func (m Model) folderPathForIndex(index int) []string {
	if m.mode == ModeRequests && index >= 0 && index < len(m.currentItemPaths) {
		return m.currentItemPaths[index]
	}
	return m.breadcrumb
}

func (m Model) folderPathForCursor() []string {
	return m.folderPathForIndex(m.cursor)
}

// toggleExpandAtCursor expands or collapses the folder under the cursor in
// the requests tree view. Requests are left untouched.
func (m Model) toggleExpandAtCursor() Model {
	if m.mode != ModeRequests || m.cursor >= len(m.currentItems) {
		return m
	}
	item := m.currentItems[m.cursor]
	if !item.IsFolder() {
		m.statusMessage = "Only folders can be expanded"
		return m
	}

	folderPath := m.folderPathForIndex(m.cursor)
	key := treeFolderKey(folderPath, item.Name)
	cursorItem := item.Name

	if m.expandedFolders[key] {
		delete(m.expandedFolders, key)
		m.statusMessage = fmt.Sprintf("Collapsed: %s", item.Name)
	} else {
		m.expandedFolders[key] = true
		m.statusMessage = fmt.Sprintf("Expanded: %s", item.Name)
	}

	current := m.currentRequestsBase()
	m = m.applyTreeItems(current, m.breadcrumb)

	for i, path := range m.currentItemPaths {
		if i < len(m.currentItems) && m.currentItems[i].Name == cursorItem && slices.Equal(path, folderPath) {
			m.cursor = i
			break
		}
	}
	if m.cursor >= len(m.currentItems) && len(m.currentItems) > 0 {
		m.cursor = len(m.currentItems) - 1
	}

	return m
}

// collectFolderKeys recursively gathers the treeFolderKey of every folder
// under items (whose parent path is parentPath) into keys.
func collectFolderKeys(items []postman.Item, parentPath []string, keys map[string]bool) {
	for _, item := range items {
		if !item.IsFolder() {
			continue
		}
		keys[treeFolderKey(parentPath, item.Name)] = true
		collectFolderKeys(item.Items, append(slices.Clone(parentPath), item.Name), keys)
	}
}

// toggleExpandAllAtCurrentLevel expands every folder in the current requests
// view (recursively, down to leaf requests) if any are collapsed, or
// collapses them all back if every folder is already expanded.
func (m Model) toggleExpandAllAtCurrentLevel() Model {
	if m.mode != ModeRequests || m.collection == nil {
		return m
	}

	base := m.currentRequestsBase()
	allKeys := make(map[string]bool)
	collectFolderKeys(base, m.breadcrumb, allKeys)

	if len(allKeys) == 0 {
		m.statusMessage = "No folders to expand"
		return m
	}

	allExpanded := true
	for key := range allKeys {
		if !m.expandedFolders[key] {
			allExpanded = false
			break
		}
	}

	if allExpanded {
		for key := range allKeys {
			delete(m.expandedFolders, key)
		}
		m.statusMessage = "Collapsed all folders"
	} else {
		for key := range allKeys {
			m.expandedFolders[key] = true
		}
		m.statusMessage = "Expanded all folders"
	}

	m = m.applyTreeItems(base, m.breadcrumb)
	if m.cursor >= len(m.currentItems) {
		m.cursor = 0
	}

	return m
}

// currentRequestsBase returns the one-level item slice for the folder the
// user is browsing (m.breadcrumb), i.e. the un-flattened base that
// applyTreeItems expands from.
func (m Model) currentRequestsBase() []postman.Item {
	if m.collection == nil {
		return nil
	}
	if len(m.breadcrumb) == 0 {
		return m.collection.Items
	}
	base := traverseToDepth(m.collection.Items, m.breadcrumb)
	if base == nil {
		return m.collection.Items
	}
	return base
}

func (m Model) itemDisplayPrefix(item postman.Item) string {
	if item.IsFolder() {
		return "[DIR] "
	}
	if requestProtocol, err := m.protocols.For(&item); err == nil {
		return fmt.Sprintf("[%s] ", requestProtocol.Label(item.Request))
	}
	return "[???] "
}

func (m Model) loadRequestsList() Model {
	if m.collection == nil {
		m.items = []string{}
		m.statusMessage = "No collection loaded"
		return m
	}

	m.breadcrumb = []string{}
	// Forces ensureSavedWorkflowIDs to re-read the workflows directory, picking up files
	// created or removed outside the TUI since the collection was last opened.
	m.savedWorkflowIDsCollection = ""

	folderCount := 0
	requestCount := 0
	otherCount := 0

	for _, item := range m.collection.Items {
		if item.IsFolder() {
			folderCount++
		} else if item.IsRequest() {
			requestCount++
		} else {
			otherCount++
		}
	}
	m = m.applyTreeItems(m.collection.Items, []string{})
	m.cursor = 0

	if len(m.items) == 0 {
		m.statusMessage = "Collection loaded but contains no items"
	} else {
		m.statusMessage = fmt.Sprintf("Loaded %d items (folders: %d, requests: %d, other: %d)",
			len(m.items), folderCount, requestCount, otherCount)
	}
	return m
}

func (m Model) loadCollection(path string) Model {
	collection, err := m.parser.LoadCollection(path)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to load collection: %v", err)
		return m
	}

	if err := m.parser.SaveState(); err != nil {
		m.statusMessage = fmt.Sprintf("Loaded collection: %s (warning: failed to save state)", collection.Info.Name)
	} else {
		m.statusMessage = fmt.Sprintf("Loaded collection: %s", collection.Info.Name)
	}

	m.collection = collection
	m.mergedCollectionActive = false
	m.mode = ModeRequests
	m = m.loadRequestsList()
	return m
}

func (m Model) handleSelection() Model {
	if len(m.items) == 0 || m.cursor >= len(m.items) {
		return m
	}

	switch m.mode {
	case ModeCollections:
		if m.parser.ActiveCollectionCount() >= 2 {
			merged := m.parser.MergedCollection()
			if merged != nil {
				m.collection = merged
				m.mergedCollectionActive = true
				m.mode = ModeRequests
				m = m.loadRequestsList()
				m.statusMessage = fmt.Sprintf("Combined view: %d collections merged (edits are saved to each request's own collection)", m.parser.ActiveCollectionCount())
			}
		} else if m.cursor < len(m.items) {
			collectionName := m.items[m.cursor]
			collection, exists := m.parser.GetCollection(collectionName)
			if exists {
				m.collection = collection
				m.mergedCollectionActive = false
				m.mode = ModeRequests
				m = m.loadRequestsList()
				m.statusMessage = fmt.Sprintf("Switched to collection: %s", collectionName)
			} else {
				m.statusMessage = fmt.Sprintf("Collection not found: %s", collectionName)
			}
		}

	case ModeRequests:
		if m.cursor < len(m.currentItems) {
			item := m.currentItems[m.cursor]
			if item.IsFolder() {
				m = m.navigateInto(item)
			}
		}
	case ModeResponse:
		m.mode = ModeRequests
		m.statusMessage = "Returned to request list"

	case ModeEnvironments:
		if m.parser.ActiveEnvironmentCount() >= 1 {
			merged := m.parser.MergedEnvironment()
			if merged != nil {
				m.environment = merged
				m.mergedEnvironmentActive = true
				m.scrollOffset = 0
				m.envVarCursor = 0
				m.previousMode = m.mode
				m.mode = ModeInfo

				m.configureViewport(&m.infoViewport, strings.Join(m.buildEnvironmentInfoLines(), "\n"))

				names := strings.Join(m.parser.ActiveEnvironments(), ", ")
				m.statusMessage = fmt.Sprintf("Showing active environment(s): %s (q to close)", names)
			}
		} else if m.cursor < len(m.items) {
			envName := m.items[m.cursor]
			environment, exists := m.parser.GetEnvironment(envName)
			if exists {
				m.environment = environment
				m.mergedEnvironmentActive = false
				m.scrollOffset = 0
				m.envVarCursor = 0
				m.previousMode = m.mode
				m.mode = ModeInfo

				m.configureViewport(&m.infoViewport, strings.Join(m.buildEnvironmentInfoLines(), "\n"))

				m.statusMessage = fmt.Sprintf("Showing environment: %s (q to close)", envName)
			} else {
				m.statusMessage = fmt.Sprintf("Environment not found: %s", envName)
			}
		}
	}

	return m
}

func (m Model) executeRequest(item postman.Item, folderPath []string) (Model, tea.Cmd) {
	requestProtocol, err := m.protocols.For(&item)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot execute: %v", err)
		return m, nil
	}

	itemID := m.requestItemID(folderPath, item.Name)
	requestToExecute := item.Request
	isModified := m.isItemModified(itemID)
	sendingStatus := "Sending"

	if isModified {
		if modifiedReq, exists := m.modifiedRequests[itemID]; exists {
			requestToExecute = modifiedReq
			sendingStatus = "Sending (unsaved)"
		}
	}
	m.statusMessage = fmt.Sprintf("%s: %s %s", sendingStatus, requestProtocol.Label(requestToExecute), item.Name)

	if stored, ok := m.requestPathParams[itemID]; ok && len(stored) > 0 {
		requestToExecute = applyPathParamsToReq(requestToExecute, stored)
	}

	collection, collectionPath := m.collectionScope(folderPath)

	m.requestExecutions[itemID] = &RequestExecution{
		Status:     statusSending,
		Timestamp:  time.Now(),
		Duration:   0,
		Response:   nil,
		TestResult: nil,
		Inputs:     sentRequestInputs(requestToExecute, collectionPath),
	}

	variables := m.variablesAt(folderPath)

	environment := m.environment
	environmentName := m.environmentName()
	itemCopy := item

	scope := &script.RequestScope{
		Collection:  collection,
		Environment: environment,
		FolderPath:  slices.Clone(collectionPath),
		Item:        &itemCopy,
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.inFlightRequests[itemID] = cancel

	return m, func() tea.Msg {
		response, testResult := requestProtocol.Execute(ctx, requestToExecute, scope, variables)

		return RequestCompleteMsg{
			Cancelled:       errors.Is(ctx.Err(), context.Canceled),
			ItemID:          itemID,
			Response:        response,
			TestResult:      testResult,
			Collection:      collection,
			Environment:     environment,
			EnvironmentName: environmentName,
			Protocol:        requestProtocol.Name(),
			ItemName:        item.Name,
			IsModified:      isModified,
		}
	}
}

func (m Model) navigateInto(item postman.Item) Model {
	return m.navigateIntoAt(item, m.breadcrumb)
}

// navigateIntoAt navigates into item as if browsing were currently at
// parentPath. This matters when item was reached via the tree-expand view
// ("z"), where item's real parent can be deeper than m.breadcrumb.
func (m Model) navigateIntoAt(item postman.Item, parentPath []string) Model {
	m.breadcrumb = append(append([]string{}, parentPath...), item.Name)
	m = m.applyTreeItems(item.Items, m.breadcrumb)
	m.cursor = 0
	m.searchActive = false
	m.searchInput.SetValue("")
	return m
}

func (m Model) navigateUp() Model {
	if len(m.breadcrumb) == 0 {
		return m
	}

	m.breadcrumb = m.breadcrumb[:len(m.breadcrumb)-1]

	if len(m.breadcrumb) == 0 {
		m = m.loadRequestsList()
	} else {
		current := traverseToDepth(m.collection.Items, m.breadcrumb)
		if current == nil {
			current = m.collection.Items
		}
		m = m.applyTreeItems(current, m.breadcrumb)
		m.cursor = 0
	}

	m.searchActive = false
	m.searchInput.SetValue("")
	return m
}

func (m Model) refreshCurrentView() Model {
	if m.collection == nil {
		return m
	}
	m.syncMergedCollection()

	if m.viewingFavorites {
		return m.applyFavoritesView()
	}

	m = m.applyTreeItems(m.currentRequestsBase(), m.breadcrumb)
	return m
}

// favoriteItemIDs returns the sorted list of favorited item IDs, suitable for
// persisting — sorted so repeated saves produce a stable diff.
func favoriteItemIDs(favorites map[string]bool) []string {
	ids := make([]string, 0, len(favorites))
	for id, on := range favorites {
		if on {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// collectFavoriteItems walks the current collection depth-first and returns
// every favorited request together with its real folder path (needed so
// execute/edit/etc. work exactly as they would browsing that folder directly).
func (m Model) collectFavoriteItems() ([]postman.Item, [][]string) {
	if m.collection == nil {
		return nil, nil
	}

	var items []postman.Item
	var paths [][]string

	var walk func(nodes []postman.Item, parentPath []string)
	walk = func(nodes []postman.Item, parentPath []string) {
		for _, item := range nodes {
			if item.IsRequest() {
				if m.favorites[m.requestItemID(parentPath, item.Name)] {
					items = append(items, item)
					paths = append(paths, slices.Clone(parentPath))
				}
			}
			if item.IsFolder() {
				walk(item.Items, append(slices.Clone(parentPath), item.Name))
			}
		}
	}
	walk(m.collection.Items, nil)

	return items, paths
}

// applyFavoritesView rebuilds m.items/currentItems/currentItemPaths as a flat
// list of every favorited request in the current collection. Because
// currentItemPaths already carries each row's real folder path independent of
// m.breadcrumb (the same mechanism the tree-expand view relies on), every
// existing ModeRequests interaction — execute, edit, script edit, duplicate,
// delete — works against these rows unmodified.
func (m Model) applyFavoritesView() Model {
	items, paths := m.collectFavoriteItems()

	display := make([]string, len(items))
	for i, item := range items {
		label := item.Name
		if len(paths[i]) > 0 {
			label = strings.Join(paths[i], " / ") + " / " + item.Name
		}
		display[i] = m.itemDisplayPrefix(item) + label
	}

	m.items = display
	m.currentItems = items
	m.currentItemPaths = paths

	if m.cursor >= len(m.currentItems) {
		if len(m.currentItems) > 0 {
			m.cursor = len(m.currentItems) - 1
		} else {
			m.cursor = 0
		}
	}

	return m
}

// enterFavoritesView switches ModeRequests into showing every favorited
// request in the current collection as a flat list.
func (m Model) enterFavoritesView() (Model, tea.Cmd) {
	if m.collection == nil {
		m.statusMessage = "No collection loaded"
		return m, nil
	}
	items, _ := m.collectFavoriteItems()
	if len(items) == 0 {
		m.statusMessage = "No favorited requests — press f on a request to favorite it"
		return m, nil
	}

	m.favoritesReturnCursor = m.cursor
	m.viewingFavorites = true
	m.mode = ModeRequests
	m.cursor = 0
	m = m.applyFavoritesView()
	m.statusMessage = fmt.Sprintf("Favorites (%d) — f to unfavorite, esc to close", len(items))
	return m, nil
}

// exitFavoritesView leaves the favorites list and restores the real folder
// the user was browsing before entering it (m.breadcrumb is never touched
// while viewingFavorites is true, so it's still accurate).
func (m Model) exitFavoritesView() Model {
	m.viewingFavorites = false
	m = m.applyTreeItems(m.currentRequestsBase(), m.breadcrumb)
	if m.favoritesReturnCursor < len(m.currentItems) {
		m.cursor = m.favoritesReturnCursor
	} else {
		m.cursor = 0
	}
	m.statusMessage = "Closed favorites view"
	return m
}

// toggleFavoriteAtCursor favorites/unfavorites the request under the cursor
// in ModeRequests (whether browsing normally or already inside the favorites
// list) and persists the full favorites set to disk.
func (m Model) toggleFavoriteAtCursor() (Model, tea.Cmd) {
	if m.mode != ModeRequests || m.collection == nil || m.cursor >= len(m.currentItems) {
		return m, nil
	}
	item := m.currentItems[m.cursor]
	if !item.IsRequest() {
		m.statusMessage = "Can only favorite requests, not folders"
		return m, nil
	}

	itemID := m.requestItemID(m.folderPathForCursor(), item.Name)
	if m.favorites == nil {
		m.favorites = make(map[string]bool)
	}

	if m.favorites[itemID] {
		delete(m.favorites, itemID)
		m.statusMessage = fmt.Sprintf("Unfavorited: %s", item.Name)
	} else {
		m.favorites[itemID] = true
		m.statusMessage = fmt.Sprintf("Favorited: %s", item.Name)
	}

	if err := postman.SaveFavorites(favoriteItemIDs(m.favorites)); err != nil {
		m.statusMessage = fmt.Sprintf("%s (failed to persist: %v)", m.statusMessage, err)
	}

	if m.viewingFavorites {
		m = m.applyFavoritesView()
	}

	return m, nil
}

func (m Model) searchItemsRecursive(items []postman.Item, query string, parentPath string) ([]string, []postman.Item, []int) {
	var displayItems []string
	var foundItems []postman.Item
	var indices []int
	query = strings.ToLower(query)

	for idx, item := range items {
		itemName := strings.ToLower(item.Name)
		fullPath := parentPath
		if fullPath != "" {
			fullPath += " / "
		}
		fullPath += item.Name

		matches := strings.Contains(itemName, query)

		if item.IsFolder() {
			subDisplay, subItems, _ := m.searchItemsRecursive(item.Items, query, fullPath)
			if len(subDisplay) > 0 {
				displayItems = append(displayItems, subDisplay...)
				foundItems = append(foundItems, subItems...)
				for range subDisplay {
					indices = append(indices, idx)
				}
			}
			if matches {
				displayItems = append(displayItems, m.itemDisplayPrefix(item)+fullPath)
				foundItems = append(foundItems, item)
				indices = append(indices, idx)
			}
		} else if item.IsRequest() && matches {
			displayItems = append(displayItems, m.itemDisplayPrefix(item)+fullPath)
			foundItems = append(foundItems, item)
			indices = append(indices, idx)
		}
	}

	return displayItems, foundItems, indices
}

func (m Model) filterItems() Model {
	searchQuery := m.searchInput.Value()
	if searchQuery == "" {
		m.items = m.allItems
		m.currentItems = m.allCurrentItems
		m.variables = m.allVariables
		m.workflows = m.allWorkflows
		m.workflowCursor = 0
		m.filteredItems = []string{}
		m.filteredIndices = []int{}
		m.cursor = 0
		return m
	}

	query := strings.ToLower(searchQuery)

	if m.mode == ModeCollections || m.mode == ModeEnvironments {
		m.filteredItems = []string{}
		m.filteredIndices = []int{}
		for idx, item := range m.allItems {
			if strings.Contains(strings.ToLower(item), query) {
				m.filteredItems = append(m.filteredItems, item)
				m.filteredIndices = append(m.filteredIndices, idx)
			}
		}
		m.items = m.filteredItems
	} else if m.mode == ModeRequests {
		if m.collection != nil {
			displayItems, foundItems, indices := m.searchItemsRecursive(m.collection.Items, searchQuery, "")
			m.filteredItems = displayItems
			m.currentItems = foundItems
			m.filteredIndices = indices
			m.items = m.filteredItems
		}
	} else if m.mode == ModeVariables {
		m.variables = filterVariables(m.allVariables, query)
		m.items = variableKeys(m.variables)
	} else if m.mode == ModeWorkflows {
		m.workflows = filterWorkflows(m.allWorkflows, query)
		m.workflowCursor = 0
	}

	m.cursor = 0
	return m
}

func (m Model) loadEnvironmentsList() Model {
	environments := m.parser.ListEnvironments()
	m.items = environments
	m.cursor = 0
	m.breadcrumb = []string{}
	m.currentItems = []postman.Item{}

	if len(m.items) == 0 {
		m.statusMessage = "No environments loaded yet. Use :loadenv <path> to load an environment"
	}
	return m
}

func (m Model) loadEnvironment(path string) Model {
	environment, err := m.parser.LoadEnvironment(path)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to load environment: %v", err)
		return m
	}

	if err := m.parser.SaveState(); err != nil {
		m.statusMessage = fmt.Sprintf("Loaded environment: %s (warning: failed to save state)", environment.Name)
	} else {
		m.statusMessage = fmt.Sprintf("Loaded environment: %s", environment.Name)
	}

	m.environment = environment
	m.mergedEnvironmentActive = false
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	return m
}

func (m Model) loadVariablesList() Model {
	m.variables = m.variablesAt(m.breadcrumb)
	m.items = variableKeys(m.variables)

	m.cursor = 0
	m.breadcrumb = []string{}
	m.currentItems = []postman.Item{}

	if len(m.items) == 0 {
		m.statusMessage = "No variables defined. Load a collection or environment with variables."
	} else {
		m.statusMessage = fmt.Sprintf("Showing %d variables", len(m.variables))
	}
	return m
}

// isViewingMergedCollection reports whether m.collection is the synthetic
// combined tree built by Parser.MergedCollection rather than a real loaded
// collection. Edits in it go to the collection owning each path (see
// owningCollectionName).
func (m Model) isViewingMergedCollection() bool {
	return m.mergedCollectionActive && m.collection != nil
}

// isViewingMergedEnvironment reports whether m.environment is the synthetic
// result of Parser.MergedEnvironment rather than a real loaded environment.
func (m Model) isViewingMergedEnvironment() bool {
	return m.mergedEnvironmentActive && m.environment != nil
}

func (m Model) enterEditMode(item postman.Item, folderPath []string) Model {
	if !item.IsRequest() || item.Request == nil {
		m.statusMessage = "Cannot edit: not a request"
		return m
	}

	m.editRequest = item.Request.Clone()
	m.editRequestSnapshot = item.Request.Clone()
	m.editItemName = item.Name
	m.editOriginalName = item.Name
	m.editFieldCursor = 0
	m.editFieldMode = false
	m.editItemPath = append([]string{}, folderPath...)
	m.previousMode = m.mode
	m.mode = ModeEdit
	m.scrollOffset = 0

	// Load stored path params for this item, or initialize empty
	itemID := m.requestItemID(folderPath, item.Name)
	m.pathParams = make(map[string]string)
	if stored, ok := m.requestPathParams[itemID]; ok {
		for k, v := range stored {
			m.pathParams[k] = v
		}
	}
	m.varSuggestions = nil
	m.varSuggestionActive = false
	m.varSuggestionCursor = 0

	if item.IsGRPC() {
		endpoint, service, method, tls := parseGRPCURL(item.Request.URL.Raw)
		m.grpcEditEndpoint = endpoint
		m.grpcEditTLS = tls
		if service != "" && method != "" {
			m.grpcEditMethod = service + "/" + method
		} else if service != "" {
			m.grpcEditMethod = service
		} else {
			m.grpcEditMethod = ""
		}
		m.editType = EditTypeGRPCRequest
		m.statusMessage = "Edit gRPC request: j/k navigate, Enter edit field, Ctrl+F reflect, :w save"
	} else {
		m.editType = EditTypeRequest
		m.statusMessage = "Edit mode: Use j/k to navigate, Enter to edit field, :w to save, :wq to save & exit"
	}

	return m
}

func (m Model) saveEdit() (Model, error) {
	if m.editType == EditTypeNone {
		m.statusMessage = "Nothing to save"
		return m, nil
	}

	switch m.editType {
	case EditTypeRequest, EditTypeGRPCRequest:
		if m.collection == nil {
			err := fmt.Errorf("no collection loaded")
			m.statusMessage = "Error: " + err.Error()
			return m, err
		}

		if m.editIsNewItem {
			items := traverseToDepthPtr(&m.collection.Items, m.editItemPath)
			if items == nil {
				err := fmt.Errorf("failed to find folder in collection")
				m.statusMessage = "Error: " + err.Error()
				return m, err
			}
			*items = append(*items, postman.Item{
				Name:    m.editItemName,
				Request: m.editRequest,
			})
			m.editIsNewItem = false
			m.editOriginalName = m.editItemName
		} else if !m.updateRequestInCollection(m.editItemPath, m.editOriginalName, m.editItemName, m.editRequest) {
			err := fmt.Errorf("failed to update request in collection")
			m.statusMessage = "Error: " + err.Error()
			return m, err
		}

		itemID := m.requestItemID(m.editItemPath, m.editOriginalName)
		m.modifiedRequests[itemID] = m.editRequest
		m.modifiedItems[itemID] = true
		m.markCollectionModified(m.editItemPath)
		m = m.saveCurrentPathParams()

		if err := m.saveOwningCollection(m.editItemPath); err != nil {
			m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
			return m, err
		}

		m.statusMessage = "Saved changes to collection file"
		delete(m.modifiedItems, itemID)
		if len(m.modifiedItems) == 0 {
			delete(m.modifiedCollections, m.owningCollectionName(m.editItemPath))
		}
	case EditTypeScript:
		m = m.saveScript()
		if err := m.saveOwningCollection(m.editItemPath); err != nil {
			m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
			return m, err
		}
		itemID := m.requestItemID(m.editItemPath, m.editScriptItemName)
		delete(m.modifiedItems, itemID)
		if len(m.modifiedItems) == 0 {
			delete(m.modifiedCollections, m.owningCollectionName(m.editItemPath))
		}
	case EditTypeWorkflowStepScript:
		m = m.saveWorkflowStepScript()
	}

	return m, nil
}

// editStateChanged reports whether the request being edited differs from the
// one edit mode was entered with.
func (m Model) editStateChanged() bool {
	return m.editItemName != m.editOriginalName || !reflect.DeepEqual(m.editRequest, m.editRequestSnapshot)
}

func (m Model) saveAllModifiedRequests() (Model, error) {
	if len(m.modifiedCollections) == 0 {
		m.statusMessage = "No unsaved changes"
		return m, nil
	}

	savedCount := 0
	var errs []string

	m.syncMergedCollection()
	for collectionName := range m.modifiedCollections {
		if err := m.saveCollectionUndoable(collectionName); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", collectionName, err))
		} else {
			savedCount++
		}
	}

	if len(errs) > 0 {
		msg := fmt.Sprintf("Saved %d collections, %d errors: %s", savedCount, len(errs), strings.Join(errs, "; "))
		m.statusMessage = msg
		return m, fmt.Errorf("%s", msg)
	}

	m.statusMessage = fmt.Sprintf("Saved %d collection(s) to file", savedCount)
	m.modifiedCollections = make(map[string]bool)
	m.modifiedItems = make(map[string]bool)
	return m, nil
}

func (m Model) handleEditModeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.requestTypeSelectionMode {
		return m.handleRequestTypeSelectionKeys(msg)
	}
	if m.scriptSelectionMode {
		return m.handleScriptSelectionKeys(msg)
	}

	if m.editType == EditTypeScript {
		return m.handleScriptEditKeys(msg)
	}

	switch msg.String() {
	case "esc":
		if m.editIsNewItem {
			m.editIsNewItem = false
			m.mode = m.previousMode
			m.editType = EditTypeNone
			m.editFieldMode = false
			m = m.refreshCurrentView()
			m.statusMessage = "New request discarded"
			return m, nil
		}

		if !m.editStateChanged() {
			m = m.saveCurrentPathParams()
			m.mode = m.previousMode
			m.editType = EditTypeNone
			m.editFieldMode = false
			m = m.refreshCurrentView()
			m.statusMessage = "No changes"
			return m, nil
		}

		if !m.updateRequestInCollection(m.editItemPath, m.editOriginalName, m.editItemName, m.editRequest) {
			m.statusMessage = "Error: Failed to update request in collection"
			return m, nil
		}

		itemID := m.requestItemID(m.editItemPath, m.editOriginalName)
		m.modifiedRequests[itemID] = m.editRequest
		m.modifiedItems[itemID] = true
		m.markCollectionModified(m.editItemPath)
		m = m.saveCurrentPathParams()
		m.mode = m.previousMode
		m.editType = EditTypeNone
		m.editFieldMode = false
		m = m.refreshCurrentView()
		m.statusMessage = "Changes saved to memory (use :w to write to file)"
		return m, nil

	case ":":
		m.commandMode = true
		m.commandInput.SetValue("")
		return m, m.commandInput.Focus()

	case "j", "down":
		fieldCount := m.getEditFieldCount()
		if m.editFieldCursor < fieldCount-1 {
			m.editFieldCursor++
		}
		return m, nil

	case "k", "up":
		if m.editFieldCursor > 0 {
			m.editFieldCursor--
		}
		return m, nil

	case "ctrl+g":
		return m.openVimForEditState()

	case requestVariablesKey:
		return handleRequestVariablesKey(m)

	case reflectKey:
		if m.editType == EditTypeGRPCRequest {
			return m.startGRPCReflection()
		}
		return m, nil

	case "enter":
		// TLS field is a boolean toggle, not a text field.
		if m.editType == EditTypeGRPCRequest && m.editFieldCursor == 5 {
			m.grpcEditTLS = !m.grpcEditTLS
			m.editRequest.URL.Raw = m.buildGRPCURL()
			tlsLabel := "Disabled (insecure)"
			if m.grpcEditTLS {
				tlsLabel = "Enabled"
			}
			if len(m.grpcReflectServices) > 0 && m.grpcEditEndpoint != "" {
				m.statusMessage = fmt.Sprintf("TLS %s — reloading reflection…", tlsLabel)
				return m.startGRPCReflection()
			}
			m.statusMessage = fmt.Sprintf("TLS %s (use :w to save)", tlsLabel)
			return m, nil
		}

		m.editFieldMode = true
		fieldValue := m.getCurrentFieldValue()
		fieldNames := m.editFieldNames()
		fieldName := fieldNames[m.editFieldCursor]

		isMultiLine := false
		if m.editType == EditTypeGRPCRequest {
			isMultiLine = m.editFieldCursor >= 3
		} else if m.editType == EditTypeRequest {
			headersIdx, _ := m.requestFieldIndices()
			isMultiLine = m.editFieldCursor >= headersIdx
		}

		if isMultiLine {
			m.editFieldTextArea.SetValue(fieldValue)
			m.editFieldTextArea.Focus()
			m.statusMessage = fmt.Sprintf("Editing %s... (Ctrl+S to save, Esc to cancel)", fieldName)
			return m, m.editFieldTextArea.Focus()
		} else {
			m.editFieldInput.SetValue(fieldValue)
			m.editFieldInput.Focus()
			m.statusMessage = fmt.Sprintf("Editing %s... (Enter to save, Tab for {{var}}, Esc to cancel)", fieldName)
			return m, m.editFieldInput.Focus()
		}
	}

	return m, nil
}

func (m Model) startGRPCReflection() (Model, tea.Cmd) {
	if m.grpcEditEndpoint == "" {
		m.statusMessage = "Set the Endpoint field before reflecting"
		return m, nil
	}
	m.statusMessage = "Connecting to gRPC server…"
	endpoint := m.grpcEditEndpoint
	tlsEnabled := m.grpcEditTLS

	return m, func() tea.Msg {
		client, err := grpc.NewClient(endpoint, tlsEnabled)
		if err != nil {
			return GRPCReflectMsg{Err: err}
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		services, err := client.ListServices(ctx)
		return GRPCReflectMsg{Services: services, Err: err}
	}
}

func (m Model) handleGRPCReflectKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case reflectKey:
		if m.grpcEditEndpoint != "" {
			m.statusMessage = "Reloading reflection…"
			return m.startGRPCReflection()
		}
		return m, nil

	case "esc":
		if m.grpcReflectPhase == GRPCPhaseMethods {
			m.grpcReflectPhase = GRPCPhaseServices
			m.statusMessage = "Navigate services with j/k, Enter to view methods, Esc to return to edit"
			return m, nil
		}
		m.mode = ModeEdit
		m.statusMessage = "Returned to edit mode"
		return m, nil

	case "j", "down":
		if m.grpcReflectPhase == GRPCPhaseServices {
			if m.grpcSelectedService < len(m.grpcReflectServices)-1 {
				m.grpcSelectedService++
			}
		} else {
			svc := m.grpcReflectServices[m.grpcSelectedService]
			if m.cursor < len(svc.Methods)-1 {
				m.cursor++
			}
		}
		return m, nil

	case "k", "up":
		if m.grpcReflectPhase == GRPCPhaseServices {
			if m.grpcSelectedService > 0 {
				m.grpcSelectedService--
			}
		} else {
			if m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil

	case "enter":
		if m.grpcReflectPhase == GRPCPhaseServices {
			if len(m.grpcReflectServices) == 0 || m.grpcSelectedService >= len(m.grpcReflectServices) {
				return m, nil
			}
			m.grpcReflectPhase = GRPCPhaseMethods
			m.cursor = 0
			svc := m.grpcReflectServices[m.grpcSelectedService]
			m.statusMessage = fmt.Sprintf("%s — %d method(s), Enter to use, Esc to go back", svc.Name, len(svc.Methods))
			return m, nil
		}

		// Phase 1: select a method
		if m.grpcSelectedService >= len(m.grpcReflectServices) {
			return m, nil
		}
		svc := m.grpcReflectServices[m.grpcSelectedService]
		if m.cursor >= len(svc.Methods) {
			return m, nil
		}
		selected := svc.Methods[m.cursor]
		m.grpcEditMethod = selected.FullMethod
		m.editRequest.URL.Raw = m.buildGRPCURL()
		if m.editRequest.Body == nil {
			m.editRequest.Body = &postman.Body{Mode: postman.BodyModeRaw}
		}
		m.editRequest.Body.Raw = selected.InputTemplate
		m.mode = ModeEdit
		m.statusMessage = "Method selected — edit message body, then :w to save"
		return m, nil
	}

	return m, nil
}

func (m Model) handleRequestTypeSelectionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.requestTypeSelectionMode = false
		m.mode = m.previousMode
		m.statusMessage = "Add request cancelled"
		return m, nil

	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}

	case "enter":
		if m.cursor < len(m.items) {
			requestType := m.items[m.cursor]
			m.requestTypeSelectionMode = false
			m = m.enterNewItemEditMode(requestType)
		}
	}

	return m, nil
}

func (m Model) enterNewItemEditMode(requestType string) Model {
	var req *postman.Request
	var editType EditType

	switch requestType {
	case "gRPC":
		req = &postman.Request{
			Method: "GRPC",
			URL:    postman.URL{Raw: "grpc://"},
			Header: []postman.Header{},
			Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: "{}"},
		}
		editType = EditTypeGRPCRequest
	case "Service Bus (ASB)":
		req = &postman.Request{
			Method: "POST",
			URL:    postman.URL{Raw: "asb://"},
			Header: []postman.Header{},
			Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: "{}"},
		}
		editType = EditTypeRequest
	case "AMQP":
		req = &postman.Request{
			Method: "POST",
			URL:    postman.URL{Raw: "amqp://"},
			Header: []postman.Header{},
			Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: "{}"},
		}
		editType = EditTypeRequest
	default: // HTTP
		req = &postman.Request{
			Method: "GET",
			URL:    postman.URL{Raw: "https://"},
			Header: []postman.Header{},
		}
		editType = EditTypeRequest
	}

	m.editRequest = req.Clone()
	m.editItemName = "New Request"
	m.editOriginalName = ""
	m.editFieldCursor = 0
	m.editFieldMode = false
	m.editItemPath = append([]string{}, m.breadcrumb...)
	m.editType = editType
	m.editIsNewItem = true
	m.previousMode = ModeRequests
	m.mode = ModeEdit
	m.scrollOffset = 0
	m.pathParams = make(map[string]string)
	m.varSuggestions = nil
	m.varSuggestionActive = false
	m.varSuggestionCursor = 0

	if editType == EditTypeGRPCRequest {
		m.grpcEditEndpoint = ""
		m.grpcEditMethod = ""
		m.grpcEditTLS = false
		m.statusMessage = "New gRPC request: set fields, :w to save, Esc to discard"
	} else {
		m.statusMessage = "New request: set fields, :w to save, Esc to discard"
	}

	return m
}

func (m Model) handleScriptSelectionKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = m.previousMode
		m.editType = EditTypeNone
		m.scriptSelectionMode = false
		m = m.refreshCurrentView()
		m.statusMessage = "Cancelled script editing"
		return m, nil

	case "j", "down":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return m, nil

	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "enter":
		if m.cursor >= len(m.items) {
			return m, nil
		}

		selection := m.items[m.cursor]

		if m.editType == EditTypeWorkflowStepScript {
			var scriptType ScriptType
			switch selection {
			case "Pre-step Script", "Create Pre-step Script":
				scriptType = ScriptTypeStepPre
			case "Post-step Script", "Create Post-step Script":
				scriptType = ScriptTypeStepPost
			default:
				m.statusMessage = "Unknown script type selected"
				return m, nil
			}
			m = m.selectWorkflowStepScriptType(scriptType)
			return m.openVimForScript()
		}

		var scriptType ScriptType
		if selection == "Pre-request Script" || selection == "Create Pre-request Script" {
			scriptType = ScriptTypePreRequest
		} else if selection == "Test Script" || selection == "Create Test Script" {
			scriptType = ScriptTypeTest
		} else {
			m.statusMessage = "Unknown script type selected"
			return m, nil
		}

		item := m.findItemByPath(m.editItemPath, m.editScriptItemName)
		if item == nil {
			m.statusMessage = "Error: Could not find request"
			return m, nil
		}

		m = m.selectScriptType(*item, scriptType)
		return m.openVimForScript()
	}

	return m, nil
}

func (m Model) handleScriptEditKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg.Type {
	case tea.KeyEsc:
		m.mode = m.previousMode
		m.editType = EditTypeNone
		m.editScript = nil
		m.scriptSelectionMode = false
		m.editFieldTextArea.Blur()
		m.editFieldTextArea.SetValue("")
		m = m.refreshCurrentView()
		m.statusMessage = "Cancelled script editing"
		return m, nil

	case tea.KeyRunes:
		if msg.String() == ":" {
			m.commandMode = true
			m.commandInput.SetValue("")
			m.commandInput.Focus()
			m.editFieldTextArea.Blur()
			return m, m.commandInput.Focus()
		}
	}

	if msg.String() == "ctrl+g" {
		return m.openVimForScript()
	}

	m.editFieldTextArea, cmd = m.editFieldTextArea.Update(msg)
	return m, cmd
}

func (m Model) handleFieldEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	isMultiLineField := false
	if m.editType == EditTypeGRPCRequest {
		isMultiLineField = m.editFieldCursor == 3 || m.editFieldCursor == 4
	} else if m.editType == EditTypeRequest && m.editRequest != nil {
		headersIdx, bodyIdx := m.requestFieldIndices()
		isMultiLineField = m.editFieldCursor == headersIdx || m.editFieldCursor == bodyIdx
	}

	switch msg.Type {
	case tea.KeyEsc:
		// Commit whatever is in the field before exiting edit mode.
		if isMultiLineField && m.editRequest != nil {
			headersIdx, bodyIdx := m.requestFieldIndices()
			switch m.editType {
			case EditTypeRequest:
				switch m.editFieldCursor {
				case headersIdx:
					m.editRequest.Header = m.parseHeaders(m.editFieldTextArea.Value())
				case bodyIdx:
					if m.editRequest.Body == nil {
						m.editRequest.Body = &postman.Body{}
					}
					m.editRequest.Body.Raw = m.editFieldTextArea.Value()
				}
			case EditTypeGRPCRequest:
				switch m.editFieldCursor {
				case 3:
					m.editRequest.Header = m.parseHeaders(m.editFieldTextArea.Value())
				case 4:
					if m.editRequest.Body == nil {
						m.editRequest.Body = &postman.Body{Mode: postman.BodyModeRaw}
					}
					m.editRequest.Body.Raw = m.editFieldTextArea.Value()
				}
			}
			m.editFieldTextArea.Blur()
			m.editFieldTextArea.SetValue("")
		} else if !isMultiLineField && m.editRequest != nil {
			headersIdx, _ := m.requestFieldIndices()
			switch m.editType {
			case EditTypeRequest:
				switch m.editFieldCursor {
				case 0:
					m.editItemName = m.editFieldInput.Value()
				case 1:
					m.editRequest.Method = m.editFieldInput.Value()
				case 2:
					m.editRequest.URL.Raw = m.editFieldInput.Value()
				default:
					params := postman.ExtractPathParams(m.editRequest.URL)
					paramIdx := m.editFieldCursor - 3
					if paramIdx >= 0 && paramIdx < len(params) && m.editFieldCursor < headersIdx {
						m.pathParams[params[paramIdx]] = m.editFieldInput.Value()
					}
				}
			case EditTypeGRPCRequest:
				switch m.editFieldCursor {
				case 0:
					m.editItemName = m.editFieldInput.Value()
				case 1:
					m.grpcEditEndpoint = m.editFieldInput.Value()
					m.editRequest.URL.Raw = m.buildGRPCURL()
				case 2:
					m.grpcEditMethod = m.editFieldInput.Value()
					m.editRequest.URL.Raw = m.buildGRPCURL()
				}
			}
			m.editFieldInput.Blur()
			m.editFieldInput.SetValue("")
		}
		m.editFieldMode = false
		m.varSuggestionActive = false
		m.varSuggestions = nil
		m.varSuggestionCursor = 0
		m.statusMessage = "Field updated (use :w to save to file)"
		return m, nil

	case tea.KeyCtrlS:
		if isMultiLineField && m.editRequest != nil {
			headersIdx, bodyIdx := m.requestFieldIndices()
			switch m.editType {
			case EditTypeRequest:
				switch m.editFieldCursor {
				case headersIdx:
					m.editRequest.Header = m.parseHeaders(m.editFieldTextArea.Value())
				case bodyIdx:
					if m.editRequest.Body == nil {
						m.editRequest.Body = &postman.Body{}
					}
					m.editRequest.Body.Raw = m.editFieldTextArea.Value()
				}
			case EditTypeGRPCRequest:
				switch m.editFieldCursor {
				case 3:
					m.editRequest.Header = m.parseHeaders(m.editFieldTextArea.Value())
				case 4:
					if m.editRequest.Body == nil {
						m.editRequest.Body = &postman.Body{Mode: postman.BodyModeRaw}
					}
					m.editRequest.Body.Raw = m.editFieldTextArea.Value()
				}
			}
			m.editFieldMode = false
			m.varSuggestionActive = false
			m.varSuggestions = nil
			m.editFieldTextArea.Blur()
			m.statusMessage = "Field updated (use :w to save to file)"
			return m, nil
		}
		return m, nil

	case tea.KeyTab:
		if m.varSuggestionActive && len(m.varSuggestions) > 0 {
			selected := m.varSuggestions[m.varSuggestionCursor]
			if isMultiLineField {
				value := m.editFieldTextArea.Value()
				idx := strings.LastIndex(value, "{{")
				if idx != -1 {
					m.editFieldTextArea.SetValue(value[:idx] + "{{" + selected.Key + "}}")
				}
			} else {
				currentVal := m.editFieldInput.Value()
				idx := strings.LastIndex(currentVal, "{{")
				if idx != -1 {
					m.editFieldInput.SetValue(currentVal[:idx] + "{{" + selected.Key + "}}")
					m.editFieldInput.CursorEnd()
				}
			}
			m.varSuggestionCursor = (m.varSuggestionCursor + 1) % len(m.varSuggestions)
			// Re-check: after inserting {{key}}, the closing }} means no active suggestion
			var checkVal string
			if isMultiLineField {
				checkVal = m.editFieldTextArea.Value()
			} else {
				checkVal = m.editFieldInput.Value()
			}
			prefix, active := detectVarPrefix(checkVal)
			m.varSuggestionActive = active
			if active {
				m.varSuggestions = m.computeVarSuggestions(prefix)
				if m.varSuggestionCursor >= len(m.varSuggestions) {
					m.varSuggestionCursor = 0
				}
			} else {
				m.varSuggestions = nil
				m.varSuggestionCursor = 0
			}
		}
		return m, nil

	case tea.KeyEnter:
		if !isMultiLineField && m.editRequest != nil {
			switch m.editType {
			case EditTypeRequest:
				headersIdx, _ := m.requestFieldIndices()
				switch m.editFieldCursor {
				case 0:
					m.editItemName = m.editFieldInput.Value()
				case 1:
					m.editRequest.Method = m.editFieldInput.Value()
				case 2:
					m.editRequest.URL.Raw = m.editFieldInput.Value()
				default:
					params := postman.ExtractPathParams(m.editRequest.URL)
					paramIdx := m.editFieldCursor - 3
					if paramIdx >= 0 && paramIdx < len(params) && m.editFieldCursor < headersIdx {
						m.pathParams[params[paramIdx]] = m.editFieldInput.Value()
					}
				}
			case EditTypeGRPCRequest:
				switch m.editFieldCursor {
				case 0:
					m.editItemName = m.editFieldInput.Value()
				case 1:
					m.grpcEditEndpoint = m.editFieldInput.Value()
					m.editRequest.URL.Raw = m.buildGRPCURL()
				case 2:
					m.grpcEditMethod = m.editFieldInput.Value()
					m.editRequest.URL.Raw = m.buildGRPCURL()
				}
			}
			m.editFieldMode = false
			m.varSuggestionActive = false
			m.varSuggestions = nil
			m.varSuggestionCursor = 0
			m.editFieldInput.Blur()
			m.statusMessage = "Field updated (use :w to save to file)"
			return m, nil
		}
		m.editFieldTextArea, cmd = m.editFieldTextArea.Update(msg)
		return m, cmd

	default:
		if isMultiLineField {
			m.editFieldTextArea, cmd = m.editFieldTextArea.Update(msg)
			prefix, active := detectVarPrefix(m.editFieldTextArea.Value())
			m.varSuggestionActive = active
			if active {
				m.varSuggestions = m.computeVarSuggestions(prefix)
				if m.varSuggestionCursor >= len(m.varSuggestions) {
					m.varSuggestionCursor = 0
				}
			} else {
				m.varSuggestions = nil
				m.varSuggestionCursor = 0
			}
		} else {
			m.editFieldInput, cmd = m.editFieldInput.Update(msg)
			prefix, active := detectVarPrefix(m.editFieldInput.Value())
			m.varSuggestionActive = active
			if active {
				m.varSuggestions = m.computeVarSuggestions(prefix)
				if m.varSuggestionCursor >= len(m.varSuggestions) {
					m.varSuggestionCursor = 0
				}
			} else {
				m.varSuggestions = nil
				m.varSuggestionCursor = 0
			}
		}
		return m, cmd
	}
}

func (m Model) getRequestIdentifierByPath(collectionName string, breadcrumb []string, requestName string) string {
	path := strings.Join(breadcrumb, "/")
	if path != "" {
		return collectionName + "/" + path + "/" + requestName
	}
	return collectionName + "/" + requestName
}

func (m Model) isItemModified(itemID string) bool {
	return m.modifiedItems[itemID]
}

func (m Model) getEditFieldCount() int {
	switch m.editType {
	case EditTypeRequest:
		if m.editRequest != nil {
			return 5 + len(postman.ExtractPathParams(m.editRequest.URL))
		}
		return 5
	case EditTypeGRPCRequest:
		return 6
	default:
		return 0
	}
}

func (m Model) editFieldNames() []string {
	if m.editType == EditTypeGRPCRequest {
		return []string{"Name", "Endpoint", "Service/Method", "Metadata", "Message", "TLS"}
	}
	names := []string{"Name", "Method", "URL"}
	if m.editRequest != nil {
		for _, name := range postman.ExtractPathParams(m.editRequest.URL) {
			names = append(names, ":"+name)
		}
	}
	names = append(names, "Headers", "Body")
	return names
}

func (m Model) requestFieldIndices() (headersIdx, bodyIdx int) {
	pathParamCount := 0
	if m.editRequest != nil {
		pathParamCount = len(postman.ExtractPathParams(m.editRequest.URL))
	}
	return 3 + pathParamCount, 4 + pathParamCount
}

func (m Model) buildGRPCURL() string {
	scheme := "grpc"
	if m.grpcEditTLS {
		scheme = "grpcs"
	}
	if m.grpcEditMethod != "" {
		return scheme + "://" + m.grpcEditEndpoint + "/" + m.grpcEditMethod
	}
	return scheme + "://" + m.grpcEditEndpoint
}

func (m Model) navigateToChangedRequest(itemID string) Model {
	parts := strings.Split(itemID, "/")
	if len(parts) < 2 {
		m.statusMessage = "Invalid request ID"
		return m
	}

	collectionName := parts[0]
	requestName := parts[len(parts)-1]
	folderPath := parts[1 : len(parts)-1]

	m, found := m.showCollectionFolder(collectionName, folderPath)
	if !found {
		m.statusMessage = fmt.Sprintf("Collection not found: %s", collectionName)
		return m
	}

	for i, item := range m.currentItems {
		if item.Name == requestName {
			m.cursor = i
			m.statusMessage = fmt.Sprintf("Navigated to: %s", requestName)
			return m
		}
	}

	m.statusMessage = fmt.Sprintf("Request not found: %s", requestName)
	return m
}

func (m Model) showChangeDiff(itemID string) Model {
	modifiedReq, hasModified := m.modifiedRequests[itemID]
	if !hasModified {
		m.statusMessage = "No modified request found"
		return m
	}

	parts := strings.Split(itemID, "/")
	if len(parts) < 2 {
		m.statusMessage = "Invalid request ID"
		return m
	}

	collectionName := parts[0]
	requestName := parts[len(parts)-1]
	folderPath := parts[1 : len(parts)-1]

	collection, exists := m.parser.GetCollection(collectionName)
	if !exists {
		m.statusMessage = "Collection not found"
		return m
	}

	originalReq := m.findOriginalRequest(collection.Items, folderPath, requestName)
	if originalReq == nil {
		m.statusMessage = "Original request not found"
		return m
	}

	m.previousMode = ModeChanges
	m.mode = ModeInfo
	m.scrollOffset = 0
	m.currentInfoItem = &postman.Item{
		Name:    "Diff: " + requestName,
		Request: modifiedReq,
	}

	m.configureViewport(&m.infoViewport, strings.Join(m.buildItemInfoLines(), "\n"))

	m.statusMessage = "Showing diff (original → modified) (q to close)"

	return m
}

func (m Model) findOriginalRequest(items []postman.Item, folderPath []string, requestName string) *postman.Request {
	current := traverseToDepth(items, folderPath)
	if current == nil {
		return nil
	}

	for _, item := range current {
		if item.IsRequest() && item.Name == requestName {
			return item.Request
		}
	}

	return nil
}

func (m Model) getCurrentFieldValue() string {
	if m.editRequest == nil {
		return ""
	}
	switch m.editType {
	case EditTypeRequest:
		headersIdx, bodyIdx := m.requestFieldIndices()
		switch m.editFieldCursor {
		case 0:
			return m.editItemName
		case 1:
			return m.editRequest.Method
		case 2:
			return m.editRequest.URL.Raw
		case headersIdx:
			return headersToText(m.editRequest.Header)
		case bodyIdx:
			if m.editRequest.Body != nil {
				return m.editRequest.Body.Raw
			}
		default:
			params := postman.ExtractPathParams(m.editRequest.URL)
			paramIdx := m.editFieldCursor - 3
			if paramIdx >= 0 && paramIdx < len(params) {
				return m.pathParams[params[paramIdx]]
			}
		}
	case EditTypeGRPCRequest:
		switch m.editFieldCursor {
		case 0:
			return m.editItemName
		case 1:
			return m.grpcEditEndpoint
		case 2:
			return m.grpcEditMethod
		case 3:
			return headersToText(m.editRequest.Header)
		case 4:
			if m.editRequest.Body != nil {
				return m.editRequest.Body.Raw
			}
		case 5:
			if m.grpcEditTLS {
				return "Enabled"
			}
			return "Disabled (insecure)"
		}
	}
	return ""
}

func headersToText(headers []postman.Header) string {
	if len(headers) == 0 {
		return ""
	}
	var lines []string
	for _, h := range headers {
		lines = append(lines, h.Key+": "+h.Value)
	}
	return strings.Join(lines, "\n")
}

func (m Model) parseHeaders(text string) []postman.Header {
	if text == "" {
		return []postman.Header{}
	}

	var headers []postman.Header
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				headers = append(headers, postman.Header{
					Key:   key,
					Value: value,
				})
			}
		}
	}
	return headers
}

func (m Model) updateRequestInCollection(path []string, originalName string, newName string, updatedRequest *postman.Request) bool {
	if m.collection == nil || updatedRequest == nil {
		return false
	}

	items := traverseToDepthPtr(&m.collection.Items, path)
	if items == nil {
		return false
	}

	for i := range *items {
		if (*items)[i].IsRequest() && (*items)[i].Name == originalName {
			(*items)[i].Name = newName
			(*items)[i].Request = updatedRequest
			return true
		}
	}

	return false
}

// insertItemAtCursor inserts item after the current cursor position (or at the end when
// search is active or the list is empty), saves the collection, and refreshes the view.
func (m Model) insertItemAtCursor(item postman.Item) Model {
	if m.collection == nil {
		m.statusMessage = "No collection loaded"
		return m
	}

	if m.isCombinedRoot(m.breadcrumb) {
		return m.blockCombinedRootEdit()
	}

	items := traverseToDepthPtr(&m.collection.Items, m.breadcrumb)
	if items == nil {
		m.statusMessage = "Error: Could not find folder"
		return m
	}

	insertIdx := len(*items)
	if !m.searchActive && m.cursor < len(*items) {
		insertIdx = m.cursor + 1
	}

	*items = append(*items, postman.Item{})
	copy((*items)[insertIdx+1:], (*items)[insertIdx:])
	(*items)[insertIdx] = item

	m.markCollectionModified(m.breadcrumb)

	if err := m.saveOwningCollection(m.breadcrumb); err != nil {
		m.statusMessage = fmt.Sprintf("Added %q but failed to save: %v", item.Name, err)
		return m
	}

	m = m.refreshCurrentView()
	m.cursor = insertIdx
	m.statusMessage = fmt.Sprintf("Added: %s", item.Name)
	return m
}

func (m Model) duplicateRequest(item postman.Item, folderPath []string) Model {
	if m.collection == nil || !item.IsRequest() || item.Request == nil {
		m.statusMessage = "Error: Cannot duplicate request"
		return m
	}

	duplicatedItem := postman.Item{
		Name:        item.Name + " (copy)",
		Request:     item.Request.Clone(),
		Description: item.Description,
	}

	items := traverseToDepthPtr(&m.collection.Items, folderPath)
	if items == nil {
		m.statusMessage = "Error: Could not find folder in breadcrumb"
		return m
	}

	*items = append(*items, duplicatedItem)

	m.markCollectionModified(folderPath)

	if err := m.saveOwningCollection(folderPath); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
		return m
	}

	m = m.refreshCurrentView()
	m.cursor = len(m.currentItems) - 1
	m.statusMessage = fmt.Sprintf("Duplicated request: %s", duplicatedItem.Name)

	return m
}

func (m Model) deleteRequest(item postman.Item, folderPath []string) Model {
	if m.collection == nil || !item.IsRequest() {
		m.statusMessage = "Error: Cannot delete request"
		return m
	}

	items := traverseToDepthPtr(&m.collection.Items, folderPath)
	if items == nil {
		m.statusMessage = "Error: Could not find folder in breadcrumb"
		return m
	}

	for i := range *items {
		if (*items)[i].Name == item.Name && (*items)[i].IsRequest() {
			*items = append((*items)[:i], (*items)[i+1:]...)
			break
		}
	}

	m.markCollectionModified(folderPath)

	if err := m.saveOwningCollection(folderPath); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
		return m
	}

	m = m.refreshCurrentView()
	if m.cursor >= len(m.currentItems) && m.cursor > 0 {
		m.cursor = len(m.currentItems) - 1
	}
	m.statusMessage = fmt.Sprintf("Deleted request: %s", item.Name)

	return m
}

func (m Model) saveSession() {
	collectionName := ""
	if m.collection != nil && !m.isViewingMergedCollection() {
		collectionName = m.collection.Info.Name
	}

	environmentName := ""
	if m.environment != nil && !m.isViewingMergedEnvironment() {
		environmentName = m.environment.Name
	}

	session := &postman.Session{
		CollectionName:  collectionName,
		EnvironmentName: environmentName,
		CombinedView:    m.isViewingMergedCollection(),
		Mode:            int(m.mode),
		Breadcrumb:      m.breadcrumb,
		Cursor:          m.cursor,
	}

	_ = m.parser.SaveSession(session)
}

func (m Model) restoreSession() Model {
	session, err := m.parser.LoadSession()
	if err != nil || session == nil {
		m.statusMessage = "No previous session found"
		return m
	}

	collectionRestored := true
	if session.CombinedView {
		if merged := m.parser.MergedCollection(); merged != nil {
			m.collection = merged
			m.mergedCollectionActive = true
		} else {
			collectionRestored = false
		}
	} else if session.CollectionName != "" {
		if collection, exists := m.parser.GetCollection(session.CollectionName); exists {
			m.collection = collection
		} else {
			collectionRestored = false
		}
	}

	if session.EnvironmentName != "" {
		if environment, exists := m.parser.GetEnvironment(session.EnvironmentName); exists {
			m.environment = environment
		}
	}

	// Active environments (marked with space, persisted across sessions) take
	// precedence over the single last-viewed environment above.
	if merged := m.parser.MergedEnvironment(); merged != nil {
		m.environment = merged
		m.mergedEnvironmentActive = true
	}

	m.mode = ViewMode(session.Mode)
	m.breadcrumb = session.Breadcrumb
	m.cursor = session.Cursor

	if m.mode == ModeRequests && m.collection == nil {
		// The last-viewed collection is gone (renamed/removed, or no
		// collections are active any more for the combined view) — fall back
		// to the collections list instead of showing an empty Requests screen.
		m.mode = ModeCollections
	}

	switch m.mode {
	case ModeCollections:
		m = m.loadCollectionsList()
	case ModeRequests:
		if len(session.Breadcrumb) == 0 {
			m = m.loadRequestsList()
		} else {
			current := traverseToDepth(m.collection.Items, session.Breadcrumb)
			if current == nil {
				m.statusMessage = "Could not restore folder path"
				m = m.loadRequestsList()
				return m
			}
			m.breadcrumb = session.Breadcrumb
			m = m.applyTreeItems(current, m.breadcrumb)
		}

		if session.Cursor >= len(m.currentItems) {
			m.cursor = 0
		} else {
			m.cursor = session.Cursor
		}
	case ModeEnvironments:
		m = m.loadEnvironmentsList()
	case ModeVariables:
		m = m.loadVariablesList()
	}

	if collectionRestored {
		m.statusMessage = "Session restored"
	} else {
		m.statusMessage = "Session restored (previous collection not found)"
	}
	return m
}

func (m Model) enterScriptSelectionMode(item postman.Item, folderPath []string) Model {
	if !item.IsRequest() {
		m.statusMessage = "Can only edit scripts for requests"
		return m
	}

	preReqCount := 0
	testCount := 0
	for _, event := range item.Events {
		if event.Listen == "prerequest" {
			preReqCount++
		} else if event.Listen == "test" {
			testCount++
		}
	}

	m.scriptSelectionMode = true
	m.editScriptItemName = item.Name
	m.editItemPath = append([]string{}, folderPath...)
	m.previousMode = m.mode
	m.mode = ModeEdit
	m.cursor = 0

	options := []string{}
	if preReqCount > 0 {
		options = append(options, "Pre-request Script")
	}
	if testCount > 0 {
		options = append(options, "Test Script")
	}
	if preReqCount == 0 {
		options = append(options, "Create Pre-request Script")
	}
	if testCount == 0 {
		options = append(options, "Create Test Script")
	}

	m.items = options
	m.statusMessage = "Select script type to edit (j/k to navigate, Enter to select, Esc to cancel)"
	return m
}

func (m Model) selectScriptType(item postman.Item, scriptType ScriptType) Model {
	var foundScript *postman.Script
	listenType := "prerequest"
	if scriptType == ScriptTypeTest {
		listenType = "test"
	}

	for i := range item.Events {
		if item.Events[i].Listen == listenType {
			foundScript = &item.Events[i].Script
			break
		}
	}

	if foundScript == nil {
		foundScript = &postman.Script{
			Type: "text/javascript",
			Exec: []string{},
		}
	}

	m.editScript = foundScript
	m.editScriptType = scriptType
	m.editType = EditTypeScript
	m.scriptSelectionMode = false

	scriptContent := ""
	if len(foundScript.Exec) > 0 {
		for i, line := range foundScript.Exec {
			scriptContent += line
			if i < len(foundScript.Exec)-1 {
				scriptContent += "\n"
			}
		}
	}

	m.editFieldTextArea.SetValue(scriptContent)
	m.editFieldTextArea.Focus()

	scriptTypeName := "pre-request"
	if scriptType == ScriptTypeTest {
		scriptTypeName = "test"
	}
	m.statusMessage = fmt.Sprintf("Editing %s script - :w to save, :wq to save & exit, Esc to cancel", scriptTypeName)
	return m
}

func (m Model) selectWorkflowStepScriptType(scriptType ScriptType) Model {
	if m.activeWorkflow == nil || m.editingWorkflowStepIdx >= len(m.activeWorkflow.Steps) {
		m.statusMessage = "Error: Step not found"
		return m
	}

	step := m.activeWorkflow.Steps[m.editingWorkflowStepIdx]
	var content string
	switch scriptType {
	case ScriptTypeStepPre:
		content = step.PreScript
	case ScriptTypeStepPost:
		content = step.PostScript
	}

	m.editScriptType = scriptType
	m.scriptSelectionMode = false
	m.editFieldTextArea.SetValue(content)
	m.editFieldTextArea.Focus()

	var scriptTypeName string
	if scriptType == ScriptTypeStepPre {
		scriptTypeName = "pre-step"
	} else {
		scriptTypeName = "post-step"
	}
	m.statusMessage = fmt.Sprintf("Editing %s script for %q — :w to save, :wq to save & exit, Esc to cancel", scriptTypeName, step.ID)
	return m
}

func (m Model) saveScript() Model {
	if m.editScript == nil || m.collection == nil {
		m.statusMessage = "Error: No script to save"
		return m
	}

	scriptContent := m.editFieldTextArea.Value()
	lines := []string{}
	if scriptContent != "" {
		for _, line := range splitLines(scriptContent) {
			lines = append(lines, line)
		}
	}

	m.editScript.Type = "text/javascript"
	m.editScript.Exec = lines

	item := m.findItemByPath(m.editItemPath, m.editScriptItemName)
	if item == nil {
		m.statusMessage = "Error: Could not find request item"
		return m
	}

	listenType := "prerequest"
	if m.editScriptType == ScriptTypeTest {
		listenType = "test"
	}

	eventFound := false
	for i := range item.Events {
		if item.Events[i].Listen == listenType {
			item.Events[i].Script = *m.editScript
			eventFound = true
			break
		}
	}

	if !eventFound {
		newEvent := postman.Event{
			Listen: listenType,
			Script: *m.editScript,
		}
		item.Events = append(item.Events, newEvent)
	}

	m.markCollectionModified(m.editItemPath)
	itemID := m.requestItemID(m.editItemPath, m.editScriptItemName)
	m.modifiedItems[itemID] = true

	scriptTypeName := "pre-request"
	if m.editScriptType == ScriptTypeTest {
		scriptTypeName = "test"
	}

	m.statusMessage = fmt.Sprintf("Saved %s script for '%s'", scriptTypeName, m.editScriptItemName)
	return m
}

func (m Model) saveActiveWorkflow() (Model, error) {
	if m.collection == nil {
		return m, fmt.Errorf("no collection loaded")
	}
	collectionPath, exists := m.parser.GetCollectionPath(m.workflowCollectionName())
	if !exists {
		return m, fmt.Errorf("collection path not found")
	}
	wfPath := workflow.WorkflowPath(collectionPath, m.activeWorkflow.ID)
	return m, workflow.SaveWorkflow(m.activeWorkflow, wfPath)
}

func (m Model) saveWorkflowStepScript() Model {
	if m.activeWorkflow == nil {
		m.statusMessage = "Error: No active workflow"
		return m
	}
	if m.editingWorkflowStepIdx < 0 || m.editingWorkflowStepIdx >= len(m.activeWorkflow.Steps) {
		m.statusMessage = "Error: Invalid step index"
		return m
	}

	content := m.editFieldTextArea.Value()
	step := &m.activeWorkflow.Steps[m.editingWorkflowStepIdx]

	var scriptTypeName string
	switch m.editScriptType {
	case ScriptTypeStepPre:
		step.PreScript = content
		scriptTypeName = "pre-step"
	case ScriptTypeStepPost:
		step.PostScript = content
		scriptTypeName = "post-step"
	default:
		m.statusMessage = "Error: Unknown workflow script type"
		return m
	}

	if m.collection == nil {
		m.statusMessage = "Error: No collection loaded"
		return m
	}
	var err error
	if m, err = m.saveActiveWorkflow(); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to save workflow: %v", err)
		return m
	}

	m.statusMessage = fmt.Sprintf("Saved %s script for step %q", scriptTypeName, step.ID)
	return m
}

func (m Model) findItemByPath(path []string, itemName string) *postman.Item {
	if m.collection == nil {
		return nil
	}

	current := traverseToDepth(m.collection.Items, path)
	if current == nil {
		return nil
	}

	for i := range current {
		if current[i].Name == itemName {
			return &current[i]
		}
	}
	return nil
}

// openWorkflowDetail sets the active workflow and enters the detail view.
func (m Model) openWorkflowDetail(wf *workflow.Workflow) (Model, tea.Cmd) {
	m.activeWorkflow = wf
	m.workflowStepCursor = 0
	m.previousMode = m.mode
	m.mode = ModeWorkflowDetail
	steps := len(wf.Steps)
	if steps == 0 {
		m.statusMessage = fmt.Sprintf("%s — no steps defined  <:wf new> to scaffold", wf.Name)
	} else {
		m.statusMessage = fmt.Sprintf("%s — %d step(s)  j/k navigate  %s/%s/%s up to/from/step  %s/R full run  enter response  %s step panel", wf.Name, steps, runUpToStepKey, runFromStepKey, runStepOnlyKey, runKey, stepPanelKey)
	}
	return m, nil
}

// startWorkflowPartial runs a contiguous range of steps [fromIdx, toIdx] without the workflow's script logic.
func (m Model) startWorkflowPartial(wf *workflow.Workflow, fromIdx, toIdx int) (Model, tea.Cmd) {
	var rangeDesc string
	switch {
	case fromIdx == toIdx:
		rangeDesc = fmt.Sprintf("step: %s", wf.Steps[fromIdx].ID)
	case fromIdx == 0:
		rangeDesc = fmt.Sprintf("up to: %s", wf.Steps[toIdx].ID)
	default:
		rangeDesc = fmt.Sprintf("from: %s", wf.Steps[fromIdx].ID)
	}
	runLabel := fmt.Sprintf("%s (%s)", wf.Name, rangeDesc)
	return m.runWorkflowScript(wf, workflow.PartialScript(wf.Steps, fromIdx, toIdx), runLabel)
}

// resolveStepItem finds the postman.Item for a workflow step by traversing the collection.
func (m Model) resolveStepItem(requestPath string) (*postman.Item, []string, error) {
	return resolveItemPath(m.workflowCollection(), requestPath)
}

// resolveItemPath finds the item at requestPath ("Folder/Request") in collection
// and returns it with its folder path.
func resolveItemPath(collection *postman.Collection, requestPath string) (*postman.Item, []string, error) {
	if collection == nil {
		return nil, nil, fmt.Errorf("no collection loaded")
	}
	parts := postman.SplitItemPath(requestPath)
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("empty request path")
	}

	folderPath := parts[:len(parts)-1]
	itemName := parts[len(parts)-1]

	current := collection.Items
	for _, folder := range folderPath {
		found := false
		for i := range current {
			if current[i].Name == folder {
				current = current[i].Items
				found = true
				break
			}
		}
		if !found {
			return nil, nil, fmt.Errorf("folder %q not found", folder)
		}
	}

	for i := range current {
		if current[i].Name == itemName {
			return &current[i], folderPath, nil
		}
	}
	return nil, nil, fmt.Errorf("request %q not found in collection", itemName)
}

// editWorkflowStepRequest navigates to the step's underlying request and enters edit mode.
func (m Model) editWorkflowStepRequest() (Model, tea.Cmd) {
	if m.activeWorkflow == nil || len(m.activeWorkflow.Steps) == 0 {
		m.statusMessage = "No step selected"
		return m, nil
	}
	if m.workflowStepCursor >= len(m.activeWorkflow.Steps) {
		m.statusMessage = "No step selected"
		return m, nil
	}

	step := m.activeWorkflow.Steps[m.workflowStepCursor]
	item, folderPath, err := m.resolveStepItem(step.Request)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot find request %q: %v", step.Request, err)
		return m, nil
	}
	if !item.IsRequest() {
		m.statusMessage = fmt.Sprintf("%q is a folder, not a request", step.Request)
		return m, nil
	}

	// Set breadcrumb so saveEdit() can locate the item in the collection.
	m.breadcrumb = m.workflowViewPath(folderPath)
	m.previousMode = ModeWorkflowDetail
	m = m.enterEditMode(*item, m.breadcrumb)
	return m, nil
}

// showWorkflowStepInfo opens the Info view for the step's underlying request.
func (m Model) showWorkflowStepInfo() (Model, tea.Cmd) {
	if m.activeWorkflow == nil || len(m.activeWorkflow.Steps) == 0 {
		m.statusMessage = "No step selected"
		return m, nil
	}
	if m.workflowStepCursor >= len(m.activeWorkflow.Steps) {
		m.statusMessage = "No step selected"
		return m, nil
	}

	step := m.activeWorkflow.Steps[m.workflowStepCursor]
	item, folderPath, err := m.resolveStepItem(step.Request)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot find request %q: %v", step.Request, err)
		return m, nil
	}

	m.currentInfoItem = item
	m.scrollOffset = 0
	m.previousMode = ModeWorkflowDetail
	m.mode = ModeInfo

	// The info sections resolve variables and dependencies at the breadcrumb,
	// which points at the requests view, not at the step's request.
	stepInfoModel := m
	stepInfoModel.breadcrumb = m.workflowViewPath(folderPath)
	lines := stepInfoModel.buildItemInfoLines()
	if ss := m.runStepState(step.ID); ss != nil {
		lines = append(lines, "")
		lines = append(lines, m.stepLogLines(ss)...)
	}
	m.configureViewport(&m.infoViewport, strings.Join(lines, "\n"))
	m.statusMessage = fmt.Sprintf("Inspecting: %s  (esc to return to workflow)", step.Request)
	return m, nil
}

// showWorkflowStepResponse opens the Response view for the step's last executed response.
func (m Model) showWorkflowStepResponse() (Model, tea.Cmd) {
	step, ok := m.selectedWorkflowStep()
	if !ok {
		m.statusMessage = "No step selected"
		return m, nil
	}
	if m.collection == nil {
		m.statusMessage = "No collection loaded"
		return m, nil
	}

	response, execution := m.selectedStepResponse()
	if response == nil {
		m.statusMessage = "No response for this step. Run the workflow first."
		return m, nil
	}

	m.lastResponse = response
	m.lastTestResult = nil
	if execution != nil {
		m.lastTestResult = execution.TestResult
	}
	m.lastExecutedItemID = m.workflowStepItemID(step.Request)
	m.responsePanelItem = nil
	m.scrollOffset = 0
	m.previousMode = ModeWorkflowDetail
	m.mode = ModeResponse

	m.configureViewport(&m.responseViewport, strings.Join(m.responseViewLines(), "\n"))
	m.statusMessage = fmt.Sprintf("Response for %s (esc to return, %s to resend the step)", step.ID, runKey)
	return m, nil
}

// deleteWorkflowStep removes the currently selected step and saves the workflow.
func (m Model) deleteWorkflowStep() (Model, tea.Cmd) {
	if m.activeWorkflow == nil || len(m.activeWorkflow.Steps) == 0 {
		m.statusMessage = "No step to delete"
		return m, nil
	}
	if m.workflowStepCursor >= len(m.activeWorkflow.Steps) {
		m.statusMessage = "No step selected"
		return m, nil
	}
	if m.collection == nil {
		m.statusMessage = "No collection loaded"
		return m, nil
	}

	deletedID := m.activeWorkflow.Steps[m.workflowStepCursor].ID
	m.activeWorkflow.Steps = append(
		m.activeWorkflow.Steps[:m.workflowStepCursor],
		m.activeWorkflow.Steps[m.workflowStepCursor+1:]...,
	)

	if m.workflowStepCursor >= len(m.activeWorkflow.Steps) && m.workflowStepCursor > 0 {
		m.workflowStepCursor--
	}

	var err error
	if m, err = m.saveActiveWorkflow(); err != nil {
		m.statusMessage = fmt.Sprintf("Deleted step %q but failed to save: %v", deletedID, err)
		return m, nil
	}

	m.statusMessage = fmt.Sprintf("Deleted step %q", deletedID)
	return m, nil
}

func (m Model) startWorkflow(wf *workflow.Workflow) (Model, tea.Cmd) {
	return m.runWorkflowScript(wf, wf.Script, "workflow: "+wf.Name)
}

// runWorkflowScript runs wf's steps with script in place of wf's own. wf stays
// the active workflow unchanged, so a partial run never replaces the full
// script that later runs and saves use.
func (m Model) runWorkflowScript(wf *workflow.Workflow, script, runLabel string) (Model, tea.Cmd) {
	if m.workflowCollection() == nil {
		m.statusMessage = "Load a collection first"
		return m, nil
	}
	if m.workflowChan != nil {
		m.statusMessage = "Workflow already running"
		return m, nil
	}

	m.activeWorkflow = wf
	m.workflowState = workflow.ExecutionState{
		WorkflowID:   wf.ID,
		WorkflowName: wf.Name,
		Status:       workflow.StatusPending,
		StepIndex:    make(map[string]*workflow.StepState),
	}
	for _, s := range wf.Steps {
		ss := &workflow.StepState{ID: s.ID, Request: s.Request, Status: workflow.StatusPending}
		m.workflowState.Steps = append(m.workflowState.Steps, ss)
		m.workflowState.StepIndex[s.ID] = ss
	}

	environment, opts, err := m.runConfig()
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot run %s: %v", runLabel, err)
		m.statusIsError = true
		return m, nil
	}

	if m.mode != ModeWorkflowDetail {
		m.previousMode = m.mode
		m.mode = ModeWorkflowDetail
		m.workflowStepCursor = 0
	}
	m.statusMessage = "Running " + runLabel
	if !m.runSettings.isDefault() {
		m.statusMessage += " (" + m.runSettings.describe() + ")"
	}

	runWf := *wf
	runWf.Script = script
	return m.launchRun(environment, func(runner *workflow.Runner, onChange func(workflow.ExecutionState)) workflow.ExecutionState {
		return runner.Run(&runWf, opts, onChange)
	})
}

// launchRun runs a workflow against environment in a goroutine, streaming
// state snapshots to the TUI until it finishes.
func (m Model) launchRun(environment *postman.Environment, run func(*workflow.Runner, func(workflow.ExecutionState)) workflow.ExecutionState) (Model, tea.Cmd) {
	progressChan := make(chan workflow.ExecutionState, workflowProgressBuffer)
	m.workflowChan = progressChan

	runner := workflow.NewRunner(m.workflowCollection(), environment, m.executor)

	return m, tea.Batch(
		func() tea.Msg {
			finalState := run(runner, func(state workflow.ExecutionState) {
				progressChan <- state
			})
			close(progressChan)
			return WorkflowMsg{State: finalState, Done: true}
		},
		waitForWorkflow(progressChan),
	)
}

func splitLines(text string) []string {
	return strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
}

// saveCurrentPathParams persists the current edit session path params into requestPathParams.
func (m Model) saveCurrentPathParams() Model {
	if len(m.pathParams) == 0 {
		return m
	}
	itemID := m.requestItemID(m.editItemPath, m.editOriginalName)
	if m.requestPathParams == nil {
		m.requestPathParams = make(map[string]map[string]string)
	}
	stored := make(map[string]string)
	for k, v := range m.pathParams {
		if v != "" {
			stored[k] = v
		}
	}
	if len(stored) > 0 {
		m.requestPathParams[itemID] = stored
	}
	return m
}

// detectVarPrefix checks if value contains an unclosed {{ and returns the partial name after it.
func detectVarPrefix(value string) (prefix string, active bool) {
	idx := strings.LastIndex(value, "{{")
	if idx == -1 {
		return "", false
	}
	afterOpen := value[idx+2:]
	if strings.Contains(afterOpen, "}}") {
		return "", false
	}
	return afterOpen, true
}

// computeVarSuggestions returns variables matching the given prefix from the current context.
func (m Model) computeVarSuggestions(prefix string) []postman.VariableSource {
	all := m.variablesAt(m.breadcrumb)
	if prefix == "" {
		return all
	}
	lowerPrefix := strings.ToLower(prefix)
	var filtered []postman.VariableSource
	for _, v := range all {
		if strings.HasPrefix(strings.ToLower(v.Key), lowerPrefix) {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// applyPathParamsToReq returns a copy of req with :paramName replaced by stored values.
func applyPathParamsToReq(req *postman.Request, params map[string]string) *postman.Request {
	if req == nil || len(params) == 0 {
		return req
	}
	modified := req.Clone()
	for name, value := range params {
		if value == "" {
			continue
		}
		modified.URL.Raw = strings.ReplaceAll(modified.URL.Raw, ":"+name, value)
		for i, seg := range modified.URL.Path {
			if seg == ":"+name {
				modified.URL.Path[i] = value
			}
		}
	}
	return modified
}
