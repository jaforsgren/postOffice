package tui

import (
	"context"
	"fmt"
	"postOffice/internal/amqp"
	"postOffice/internal/grpc"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/servicebus"
	"postOffice/internal/workflow"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.responseViewport.Width = msg.Width - viewportPadding
		m.infoViewport.Width = msg.Width - viewportPadding
		m.jsonViewport.Width = msg.Width - viewportPadding
		return m, nil

	case RequestCompleteMsg:
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
		}

		if m.mode == ModeVariables {
			m = m.loadVariablesList()
		}

		if msg.Environment != nil && msg.TestResult != nil {
			if err := m.parser.SaveEnvironment(msg.Environment.Name); err != nil {
				m.statusMessage = fmt.Sprintf("Warning: failed to save environment variables: %v", err)
			}
		}

		if msg.Response.Error != nil {
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

		if m.mode == ModeResponse {
			m.configureViewport(&m.responseViewport, strings.Join(m.buildResponseLines(), "\n"))
		}

		return m, nil

	case WorkflowMsg:
		m.workflowState = msg.State
		if msg.Done {
			if msg.State.Status == workflow.StatusSuccess {
				m.statusMessage = fmt.Sprintf("Workflow %q completed", msg.State.WorkflowName)
			} else {
				errMsg := msg.State.Error
				if errMsg == "" {
					errMsg = msg.State.Status
				}
				m.statusMessage = fmt.Sprintf("Workflow %q %s: %s", msg.State.WorkflowName, msg.State.Status, errMsg)
			}
			m.workflowChan = nil
			if m.collection != nil {
				for _, ss := range msg.State.Steps {
					if ss.LastResponse != nil {
						itemID := m.collection.Info.Name + "/" + ss.Request
						m.requestExecutions[itemID] = &RequestExecution{
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

	case tea.KeyMsg:
		if m.commandMode {
			return m.handleCommandMode(msg)
		}
		if m.searchMode {
			return m.handleSearchMode(msg)
		}
		if m.mode == ModeGRPCReflect {
			return m.handleGRPCReflectKeys(msg)
		}
		if m.mode == ModeEdit {
			if m.editFieldMode {
				return m.handleFieldEdit(msg)
			}
			return m.handleEditModeKeys(msg)
		}

		if m.mode == ModeResponse || m.mode == ModeInfo || m.mode == ModeJSON || m.mode == ModeHelp {
			key := msg.String()
			if key == "esc" || key == "h" || key == "backspace" || key == "q" {
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
			}
			return m, cmd
		}

		return m.handleNormalMode(msg)
	}

	return m, tea.Batch(cmds...)
}

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
		m.searchInput.SetValue("")
		m.searchInput.Blur()
		m.searchActive = false
		m.items = m.allItems
		m.currentItems = m.allCurrentItems
		m.cursor = 0
		m.statusMessage = "Search cancelled"
		return m, nil

	case tea.KeyEnter:
		m.searchMode = false
		m.searchInput.Blur()
		m.searchActive = len(m.items) > 0 && m.searchInput.Value() != ""
		if len(m.items) > 0 {
			m.statusMessage = fmt.Sprintf("Found %d results (press Esc to clear search)", len(m.items))
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

// buildDisplayList converts a slice of items into display strings with method/type prefixes.
func buildDisplayList(items []postman.Item) []string {
	list := make([]string, len(items))
	for i, item := range items {
		list[i] = itemDisplayPrefix(item) + item.Name
	}
	return list
}

func itemDisplayPrefix(item postman.Item) string {
	if item.IsFolder() {
		return "[DIR] "
	}
	if item.IsServiceBus() {
		return "[ASB] "
	}
	if item.IsAMQP() {
		return "[AMQP] "
	}
	if item.IsRequest() {
		return fmt.Sprintf("[%s] ", item.Request.Method)
	}
	return "[???] "
}

func (m Model) loadRequestsList() Model {
	if m.collection == nil {
		m.items = []string{}
		m.statusMessage = "No collection loaded"
		return m
	}

	m.items = []string{}
	m.currentItems = m.collection.Items
	m.breadcrumb = []string{}

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
	m.items = buildDisplayList(m.collection.Items)
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
		if m.cursor < len(m.items) {
			collectionName := m.items[m.cursor]
			collection, exists := m.parser.GetCollection(collectionName)
			if exists {
				m.collection = collection
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
		if m.cursor < len(m.items) {
			envName := m.items[m.cursor]
			environment, exists := m.parser.GetEnvironment(envName)
			if exists {
				m.environment = environment
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

func (m Model) executeRequest(item postman.Item) (Model, tea.Cmd) {
	if item.IsGRPC() {
		m.statusMessage = "gRPC execution not yet supported — use :info to inspect the request"
		return m, nil
	}
	if !item.IsRequest() || item.Request == nil {
		m.statusMessage = "Cannot execute: not a request"
		return m, nil
	}

	if item.IsServiceBus() {
		return m.executeServiceBusRequest(item)
	}

	if item.IsAMQP() {
		return m.executeAMQPRequest(item)
	}

	itemID := m.getRequestIdentifier(item)
	requestToExecute := item.Request
	isModified := m.isItemModified(itemID)

	if isModified {
		if modifiedReq, exists := m.modifiedRequests[itemID]; exists {
			requestToExecute = modifiedReq
			m.statusMessage = fmt.Sprintf("Sending (unsaved): %s %s", requestToExecute.Method, item.Name)
		} else {
			m.statusMessage = fmt.Sprintf("Sending: %s %s", item.Request.Method, item.Name)
		}
	} else {
		m.statusMessage = fmt.Sprintf("Sending: %s %s", item.Request.Method, item.Name)
	}

	if stored, ok := m.requestPathParams[itemID]; ok && len(stored) > 0 {
		requestToExecute = applyPathParamsToReq(requestToExecute, stored)
	}

	m.requestExecutions[itemID] = &RequestExecution{
		Status:     "Sending...",
		Timestamp:  time.Now(),
		Duration:   0,
		Response:   nil,
		TestResult: nil,
	}

	variables := postman.GetAllVariables(m.collection, m.breadcrumb, m.environment)

	executor := m.executor
	collection := m.collection
	environment := m.environment
	itemCopy := item

	return m, func() tea.Msg {
		response, testResult := executor.Execute(requestToExecute, &itemCopy, collection, environment, variables)

		return RequestCompleteMsg{
			ItemID:      itemID,
			Response:    response,
			TestResult:  testResult,
			Collection:  collection,
			Environment: environment,
			ItemName:    item.Name,
			IsModified:  isModified,
		}
	}
}

func (m Model) executeServiceBusRequest(item postman.Item) (Model, tea.Cmd) {
	return m.executeMessageItem(item, "Sending ASB message", func(req *postman.Request, vars []postman.VariableSource) *internalhttp.Response {
		return servicebus.Send(req, vars)
	})
}

func (m Model) executeAMQPRequest(item postman.Item) (Model, tea.Cmd) {
	return m.executeMessageItem(item, "Publishing AMQP message", func(req *postman.Request, vars []postman.VariableSource) *internalhttp.Response {
		return amqp.Publish(req, vars)
	})
}

func (m Model) executeMessageItem(
	item postman.Item,
	statusPrefix string,
	send func(*postman.Request, []postman.VariableSource) *internalhttp.Response,
) (Model, tea.Cmd) {
	itemID := m.getRequestIdentifier(item)
	m.statusMessage = fmt.Sprintf("%s: %s", statusPrefix, item.Name)
	m.requestExecutions[itemID] = &RequestExecution{
		Status:    "Sending...",
		Timestamp: time.Now(),
	}

	collection := m.collection
	environment := m.environment
	itemCopy := item

	return m, func() tea.Msg {
		// Run pre-request scripts so they can set variables (e.g. generate IDs).
		if len(itemCopy.Events) > 0 {
			ctx := &script.ExecutionContext{}
			if collection != nil {
				ctx.CollectionVars = collection.Variables
			}
			if environment != nil {
				ctx.EnvironmentVars = environment.Values
			}
			_ = script.ExecutePreRequestScripts(itemCopy.Events, ctx)
			if collection != nil {
				collection.Variables = ctx.CollectionVars
			}
			if environment != nil {
				environment.Values = ctx.EnvironmentVars
			}
		}

		variables := postman.GetAllVariables(collection, nil, environment)
		resp := send(itemCopy.Request, variables)
		return RequestCompleteMsg{
			ItemID:      itemID,
			Response:    resp,
			Collection:  collection,
			Environment: environment,
			ItemName:    itemCopy.Name,
		}
	}
}

func (m Model) navigateInto(item postman.Item) Model {
	m.breadcrumb = append(m.breadcrumb, item.Name)
	m.currentItems = item.Items
	m.items = buildDisplayList(item.Items)
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
		m.currentItems = current
		m.items = buildDisplayList(current)
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

	if len(m.breadcrumb) == 0 {
		m.currentItems = m.collection.Items
	} else {
		current := traverseToDepth(m.collection.Items, m.breadcrumb)
		if current == nil {
			current = m.collection.Items
		}
		m.currentItems = current
	}

	m.items = buildDisplayList(m.currentItems)
	return m
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
				displayItems = append(displayItems, itemDisplayPrefix(item)+fullPath)
				foundItems = append(foundItems, item)
				indices = append(indices, idx)
			}
		} else if item.IsRequest() && matches {
			displayItems = append(displayItems, itemDisplayPrefix(item)+fullPath)
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
	m.mode = ModeEnvironments
	m = m.loadEnvironmentsList()
	return m
}

func (m Model) loadVariablesList() Model {
	m.variables = postman.GetAllVariables(m.collection, m.breadcrumb, m.environment)

	m.items = []string{}
	for _, variable := range m.variables {
		m.items = append(m.items, variable.Key)
	}

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

func (m Model) enterEditMode(item postman.Item) Model {
	if !item.IsRequest() || item.Request == nil {
		m.statusMessage = "Cannot edit: not a request"
		return m
	}

	m.editRequest = m.deepCopyRequest(item.Request)
	m.editItemName = item.Name
	m.editOriginalName = item.Name
	m.editFieldCursor = 0
	m.editFieldMode = false
	m.editCollectionName = m.collection.Info.Name
	m.editItemPath = append([]string{}, m.breadcrumb...)
	m.previousMode = m.mode
	m.mode = ModeEdit
	m.scrollOffset = 0

	// Load stored path params for this item, or initialize empty
	itemID := m.getRequestIdentifier(item)
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
		m.statusMessage = "Edit gRPC request: j/k navigate, Enter edit field, Ctrl+R reflect, :w save"
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

		itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editOriginalName)
		m.modifiedRequests[itemID] = m.editRequest
		m.modifiedItems[itemID] = true
		m.modifiedCollections[m.editCollectionName] = true
		m = m.saveCurrentPathParams()

		if err := m.parser.SaveCollection(m.editCollectionName); err != nil {
			m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
			return m, err
		}

		m.statusMessage = "Saved changes to collection file"
		delete(m.modifiedItems, itemID)
		if len(m.modifiedItems) == 0 {
			delete(m.modifiedCollections, m.editCollectionName)
		}
	case EditTypeScript:
		m = m.saveScript()
		if err := m.parser.SaveCollection(m.editCollectionName); err != nil {
			m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
			return m, err
		}
		itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editScriptItemName)
		delete(m.modifiedItems, itemID)
		if len(m.modifiedItems) == 0 {
			delete(m.modifiedCollections, m.editCollectionName)
		}
	case EditTypeWorkflowStepScript:
		m = m.saveWorkflowStepScript()
	}

	return m, nil
}

func (m Model) saveAllModifiedRequests() (Model, error) {
	if len(m.modifiedCollections) == 0 {
		m.statusMessage = "No unsaved changes"
		return m, nil
	}

	savedCount := 0
	var errs []string

	for collectionName := range m.modifiedCollections {
		if err := m.parser.SaveCollection(collectionName); err != nil {
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

		if !m.updateRequestInCollection(m.editItemPath, m.editOriginalName, m.editItemName, m.editRequest) {
			m.statusMessage = "Error: Failed to update request in collection"
			return m, nil
		}

		itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editOriginalName)
		m.modifiedRequests[itemID] = m.editRequest
		m.modifiedItems[itemID] = true
		m.modifiedCollections[m.editCollectionName] = true
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

	case "ctrl+r":
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
	case "ctrl+r":
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
			m.editRequest.Body = &postman.Body{Mode: "raw"}
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
			Body:   &postman.Body{Mode: "raw", Raw: "{}"},
		}
		editType = EditTypeGRPCRequest
	case "Service Bus (ASB)":
		req = &postman.Request{
			Method: "POST",
			URL:    postman.URL{Raw: "asb://"},
			Header: []postman.Header{},
			Body:   &postman.Body{Mode: "raw", Raw: "{}"},
		}
		editType = EditTypeRequest
	case "AMQP":
		req = &postman.Request{
			Method: "POST",
			URL:    postman.URL{Raw: "amqp://"},
			Header: []postman.Header{},
			Body:   &postman.Body{Mode: "raw", Raw: "{}"},
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

	m.editRequest = m.deepCopyRequest(req)
	m.editItemName = "New Request"
	m.editOriginalName = ""
	m.editFieldCursor = 0
	m.editFieldMode = false
	m.editCollectionName = m.collection.Info.Name
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
			return m, nil
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
		return m, nil
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
						m.editRequest.Body = &postman.Body{Mode: "raw"}
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
						m.editRequest.Body = &postman.Body{Mode: "raw"}
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

func (m Model) deepCopyRequest(req *postman.Request) *postman.Request {
	if req == nil {
		return nil
	}

	copied := &postman.Request{
		Method: req.Method,
	}

	copied.URL.Raw = req.URL.Raw
	copied.URL.Port = req.URL.Port
	copied.URL.Protocol = req.URL.Protocol
	if req.URL.Host != nil {
		copied.URL.Host = make([]string, len(req.URL.Host))
		copy(copied.URL.Host, req.URL.Host)
	}
	if req.URL.Path != nil {
		copied.URL.Path = make([]string, len(req.URL.Path))
		copy(copied.URL.Path, req.URL.Path)
	}

	if req.Header != nil {
		copied.Header = make([]postman.Header, len(req.Header))
		copy(copied.Header, req.Header)
	}

	if req.Body != nil {
		copied.Body = &postman.Body{
			Mode: req.Body.Mode,
			Raw:  req.Body.Raw,
		}
	}

	return copied
}

func (m Model) getRequestIdentifier(item postman.Item) string {
	if m.collection == nil {
		return ""
	}
	path := strings.Join(m.breadcrumb, "/")
	if path != "" {
		return m.collection.Info.Name + "/" + path + "/" + item.Name
	}
	return m.collection.Info.Name + "/" + item.Name
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

	if m.collection == nil || m.collection.Info.Name != collectionName {
		if collection, exists := m.parser.GetCollection(collectionName); exists {
			m.collection = collection
		} else {
			m.statusMessage = fmt.Sprintf("Collection not found: %s", collectionName)
			return m
		}
	}

	m.mode = ModeRequests
	m.breadcrumb = folderPath
	m = m.loadRequestsList()

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

	if m.collection == nil || m.collection.Info.Name != collectionName {
		if collection, exists := m.parser.GetCollection(collectionName); exists {
			m.collection = collection
		} else {
			m.statusMessage = "Collection not found"
			return m
		}
	}

	originalReq := m.findOriginalRequest(m.collection.Items, folderPath, requestName)
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

	m.modifiedCollections[m.collection.Info.Name] = true

	if err := m.parser.SaveCollection(m.collection.Info.Name); err != nil {
		m.statusMessage = fmt.Sprintf("Added %q but failed to save: %v", item.Name, err)
		return m
	}

	m = m.refreshCurrentView()
	m.cursor = insertIdx
	m.statusMessage = fmt.Sprintf("Added: %s", item.Name)
	return m
}

func (m Model) duplicateRequest(item postman.Item) Model {
	if m.collection == nil || !item.IsRequest() || item.Request == nil {
		m.statusMessage = "Error: Cannot duplicate request"
		return m
	}

	duplicatedItem := postman.Item{
		Name:        item.Name + " (copy)",
		Request:     m.deepCopyRequest(item.Request),
		Description: item.Description,
	}

	items := traverseToDepthPtr(&m.collection.Items, m.breadcrumb)
	if items == nil {
		m.statusMessage = "Error: Could not find folder in breadcrumb"
		return m
	}

	*items = append(*items, duplicatedItem)

	m.modifiedCollections[m.collection.Info.Name] = true

	if err := m.parser.SaveCollection(m.collection.Info.Name); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
		return m
	}

	m = m.refreshCurrentView()
	m.cursor = len(m.currentItems) - 1
	m.statusMessage = fmt.Sprintf("Duplicated request: %s", duplicatedItem.Name)

	return m
}

func (m Model) deleteRequest(item postman.Item) Model {
	if m.collection == nil || !item.IsRequest() {
		m.statusMessage = "Error: Cannot delete request"
		return m
	}

	items := traverseToDepthPtr(&m.collection.Items, m.breadcrumb)
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

	m.modifiedCollections[m.collection.Info.Name] = true

	if err := m.parser.SaveCollection(m.collection.Info.Name); err != nil {
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
	if m.collection != nil {
		collectionName = m.collection.Info.Name
	}

	environmentName := ""
	if m.environment != nil {
		environmentName = m.environment.Name
	}

	session := &postman.Session{
		CollectionName:  collectionName,
		EnvironmentName: environmentName,
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

	if session.CollectionName != "" {
		if collection, exists := m.parser.GetCollection(session.CollectionName); exists {
			m.collection = collection
		} else {
			m.statusMessage = "Previous session's collection not found"
			return m
		}
	}

	if session.EnvironmentName != "" {
		if environment, exists := m.parser.GetEnvironment(session.EnvironmentName); exists {
			m.environment = environment
		}
	}

	m.mode = ViewMode(session.Mode)
	m.breadcrumb = session.Breadcrumb
	m.cursor = session.Cursor

	switch m.mode {
	case ModeCollections:
		m = m.loadCollectionsList()
	case ModeRequests:
		if m.collection != nil {
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
				m.currentItems = current
				m.items = buildDisplayList(current)
			}

			if session.Cursor >= len(m.currentItems) {
				m.cursor = 0
			} else {
				m.cursor = session.Cursor
			}
		}
	case ModeEnvironments:
		m = m.loadEnvironmentsList()
	case ModeVariables:
		m = m.loadVariablesList()
	}

	m.statusMessage = "Session restored"
	return m
}

func (m Model) enterScriptSelectionMode(item postman.Item) Model {
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
	m.editItemPath = append([]string{}, m.breadcrumb...)
	m.editCollectionName = m.collection.Info.Name
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

	m.modifiedCollections[m.editCollectionName] = true
	itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editScriptItemName)
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
	collectionPath, exists := m.parser.GetCollectionPath(m.collection.Info.Name)
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
		m.statusMessage = fmt.Sprintf("%s — %d step(s)  j/k navigate  u/f/r partial run  R full run  ctrl+r view response", wf.Name, steps)
	}
	return m, nil
}

// startWorkflowPartial runs a contiguous range of steps [fromIdx, toIdx] without the workflow's script logic.
func (m Model) startWorkflowPartial(wf *workflow.Workflow, fromIdx, toIdx int) (Model, tea.Cmd) {
	if m.collection == nil {
		m.statusMessage = "Load a collection first"
		return m, nil
	}

	partialWf := *wf
	partialWf.Script = workflow.PartialScript(wf.Steps, fromIdx, toIdx)

	var rangeDesc string
	switch {
	case fromIdx == toIdx:
		rangeDesc = fmt.Sprintf("step: %s", wf.Steps[fromIdx].ID)
	case fromIdx == 0:
		rangeDesc = fmt.Sprintf("up to: %s", wf.Steps[toIdx].ID)
	default:
		rangeDesc = fmt.Sprintf("from: %s", wf.Steps[fromIdx].ID)
	}
	m.statusMessage = fmt.Sprintf("Running %s (%s)", wf.Name, rangeDesc)

	return m.startWorkflow(&partialWf)
}

// resolveStepItem finds the postman.Item for a workflow step by traversing the collection.
func (m Model) resolveStepItem(requestPath string) (*postman.Item, []string, error) {
	if m.collection == nil {
		return nil, nil, fmt.Errorf("no collection loaded")
	}
	parts := strings.Split(requestPath, "/")
	if len(parts) == 0 {
		return nil, nil, fmt.Errorf("empty request path")
	}

	folderPath := parts[:len(parts)-1]
	itemName := parts[len(parts)-1]

	current := m.collection.Items
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
	m.breadcrumb = folderPath
	m.previousMode = ModeWorkflowDetail
	m = m.enterEditMode(*item)
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
	item, _, err := m.resolveStepItem(step.Request)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Cannot find request %q: %v", step.Request, err)
		return m, nil
	}

	m.currentInfoItem = item
	m.scrollOffset = 0
	m.previousMode = ModeWorkflowDetail
	m.mode = ModeInfo

	m.configureViewport(&m.infoViewport, strings.Join(m.buildItemInfoLines(), "\n"))
	m.statusMessage = fmt.Sprintf("Inspecting: %s  (esc to return to workflow)", step.Request)
	return m, nil
}

// showWorkflowStepResponse opens the Response view for the step's last executed response.
func (m Model) showWorkflowStepResponse() (Model, tea.Cmd) {
	if m.activeWorkflow == nil || len(m.activeWorkflow.Steps) == 0 {
		m.statusMessage = "No step selected"
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

	step := m.activeWorkflow.Steps[m.workflowStepCursor]
	itemID := m.collection.Info.Name + "/" + step.Request

	exec, exists := m.requestExecutions[itemID]
	if !exists || exec.Response == nil {
		m.statusMessage = "No response for this step. Run the workflow first."
		return m, nil
	}

	m.lastResponse = exec.Response
	m.lastTestResult = exec.TestResult
	m.lastExecutedItemID = itemID
	m.scrollOffset = 0
	m.previousMode = ModeWorkflowDetail
	m.mode = ModeResponse

	m.configureViewport(&m.responseViewport, strings.Join(m.buildResponseLines(), "\n"))
	m.statusMessage = fmt.Sprintf("Response for %s (esc to return)", step.ID)
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
	if m.collection == nil {
		m.statusMessage = "Load a collection first"
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

	if m.workflowChan != nil {
		m.statusMessage = "Workflow already running"
		return m, nil
	}

	if m.mode != ModeWorkflowDetail {
		m.previousMode = m.mode
		m.mode = ModeWorkflowDetail
		m.workflowStepCursor = 0
	}
	m.statusMessage = fmt.Sprintf("Running workflow: %s", wf.Name)

	progressChan := make(chan workflow.ExecutionState, 20)
	m.workflowChan = progressChan

	collection := m.collection
	environment := m.environment
	executor := m.executor

	return m, tea.Batch(
		// Run the workflow in a goroutine; close the channel when done.
		func() tea.Msg {
			runner := workflow.NewRunner(collection, environment, executor)
			finalState := runner.Run(wf, func(state workflow.ExecutionState) {
				progressChan <- state
			})
			close(progressChan)
			return WorkflowMsg{State: finalState, Done: true}
		},
		// Start receiving progress updates.
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
	itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editOriginalName)
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
	all := postman.GetAllVariables(m.collection, m.breadcrumb, m.environment)
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
	modified := &postman.Request{
		Method: req.Method,
		URL: postman.URL{
			Raw:      req.URL.Raw,
			Port:     req.URL.Port,
			Protocol: req.URL.Protocol,
		},
	}
	if req.URL.Host != nil {
		modified.URL.Host = make([]string, len(req.URL.Host))
		copy(modified.URL.Host, req.URL.Host)
	}
	if req.URL.Path != nil {
		modified.URL.Path = make([]string, len(req.URL.Path))
		copy(modified.URL.Path, req.URL.Path)
	}
	if req.Header != nil {
		modified.Header = make([]postman.Header, len(req.Header))
		copy(modified.Header, req.Header)
	}
	if req.Body != nil {
		modified.Body = &postman.Body{Mode: req.Body.Mode, Raw: req.Body.Raw}
	}
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
