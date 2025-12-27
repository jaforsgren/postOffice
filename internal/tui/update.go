package tui

import (
	"fmt"
	"postOffice/internal/postman"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.responseViewport.Width = msg.Width - 8
		m.infoViewport.Width = msg.Width - 8
		m.jsonViewport.Width = msg.Width - 8
		return m, nil

	case tea.KeyMsg:
		if m.commandMode {
			return m.handleCommandMode(msg)
		}
		if m.searchMode {
			return m.handleSearchMode(msg)
		}
		if m.mode == ModeEdit {
			if m.editFieldMode {
				return m.handleFieldEdit(msg)
			}
			return m.handleEditModeKeys(msg)
		}

		if m.mode == ModeResponse || m.mode == ModeInfo || m.mode == ModeJSON {
			key := msg.String()
			if key == "esc" || key == "h" || key == "backspace" || key == "q" {
				return m.handleNormalMode(msg)
			}

			if m.mode == ModeResponse {
				m.responseViewport, cmd = m.responseViewport.Update(msg)
				return m, cmd
			} else if m.mode == ModeInfo {
				m.infoViewport, cmd = m.infoViewport.Update(msg)
				return m, cmd
			} else if m.mode == ModeJSON {
				m.jsonViewport, cmd = m.jsonViewport.Update(msg)
				return m, cmd
			}
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

	if key == ":" {
		m.commandMode = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
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
	if input == "" || strings.Contains(input, " ") {
		return ""
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
		prefix := ""
		if item.IsFolder() {
			prefix = "[DIR] "
			folderCount++
		} else if item.IsRequest() {
			prefix = fmt.Sprintf("[%s] ", item.Request.Method)
			requestCount++
		} else {
			prefix = "[???] "
			otherCount++
		}
		m.items = append(m.items, prefix+item.Name)
	}
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
			} else if item.IsRequest() {
				itemID := m.getRequestIdentifier(item)
				requestToExecute := item.Request

				if m.isItemModified(itemID) {
					if modifiedReq, exists := m.modifiedRequests[itemID]; exists {
						requestToExecute = modifiedReq
						m.statusMessage = fmt.Sprintf("Executing (unsaved): %s %s", requestToExecute.Method, item.Name)
					} else {
						m.statusMessage = fmt.Sprintf("Executing: %s %s", item.Request.Method, item.Name)
					}
				} else {
					m.statusMessage = fmt.Sprintf("Executing: %s %s", item.Request.Method, item.Name)
				}

				variables := m.parser.GetAllVariables(m.collection, m.breadcrumb, m.environment)
				m.lastResponse, m.lastTestResult = m.executor.Execute(requestToExecute, &item, m.collection, m.environment, variables)
				m.scrollOffset = 0
				m.mode = ModeResponse

				metrics := m.calculateSplitLayout()
				m.responseViewport.Width = m.width - 8
				m.responseViewport.Height = metrics.popupHeight - 4
				lines := m.buildResponseLines()
				content := strings.Join(lines, "\n")
				m.responseViewport.SetContent(content)

				if m.lastResponse.Error != nil {
					m.statusMessage = fmt.Sprintf("Request failed: %v", m.lastResponse.Error)
				} else {
					statusSuffix := ""
					if m.isItemModified(itemID) {
						statusSuffix = " [unsaved changes]"
					}
					m.statusMessage = fmt.Sprintf("Response: %s (%v)%s", m.lastResponse.Status, m.lastResponse.Duration, statusSuffix)
				}
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

				metrics := m.calculateSplitLayout()
				m.infoViewport.Width = m.width - 8
				m.infoViewport.Height = metrics.popupHeight - 4
				lines := m.buildEnvironmentInfoLines()
				content := strings.Join(lines, "\n")
				m.infoViewport.SetContent(content)

				m.statusMessage = fmt.Sprintf("Showing environment: %s", envName)
			} else {
				m.statusMessage = fmt.Sprintf("Environment not found: %s", envName)
			}
		}
	}

	return m
}

func (m Model) navigateInto(item postman.Item) Model {
	m.breadcrumb = append(m.breadcrumb, item.Name)
	m.currentItems = item.Items
	m.items = []string{}
	for _, subItem := range item.Items {
		prefix := ""
		if subItem.IsFolder() {
			prefix = "[DIR] "
		} else if subItem.IsRequest() {
			prefix = fmt.Sprintf("[%s] ", subItem.Request.Method)
		} else {
			prefix = "[???] "
		}
		m.items = append(m.items, prefix+subItem.Name)
	}
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
		current := m.collection.Items
		for _, crumb := range m.breadcrumb {
			for _, item := range current {
				if item.Name == crumb {
					current = item.Items
					break
				}
			}
		}
		m.currentItems = current
		m.items = []string{}
		for _, item := range current {
			prefix := ""
			if item.IsFolder() {
				prefix = "[DIR] "
			} else if item.IsRequest() {
				prefix = fmt.Sprintf("[%s] ", item.Request.Method)
			} else {
				prefix = "[???] "
			}
			m.items = append(m.items, prefix+item.Name)
		}
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
		current := m.collection.Items
		for _, crumb := range m.breadcrumb {
			for _, item := range current {
				if item.Name == crumb {
					current = item.Items
					break
				}
			}
		}
		m.currentItems = current
	}

	m.items = []string{}
	for _, item := range m.currentItems {
		prefix := ""
		if item.IsFolder() {
			prefix = "[DIR] "
		} else if item.IsRequest() {
			prefix = fmt.Sprintf("[%s] ", item.Request.Method)
		} else {
			prefix = "[???] "
		}
		m.items = append(m.items, prefix+item.Name)
	}

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
				prefix := "[DIR] "
				displayItems = append(displayItems, prefix+fullPath)
				foundItems = append(foundItems, item)
				indices = append(indices, idx)
			}
		} else if item.IsRequest() && matches {
			prefix := fmt.Sprintf("[%s] ", item.Request.Method)
			displayItems = append(displayItems, prefix+fullPath)
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
	m.variables = m.parser.GetAllVariables(m.collection, m.breadcrumb, m.environment)

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
	m.editType = EditTypeRequest
	m.editFieldCursor = 0
	m.editFieldMode = false
	m.editCollectionName = m.collection.Info.Name
	m.editItemPath = append([]string{}, m.breadcrumb...)
	m.previousMode = m.mode
	m.mode = ModeEdit
	m.scrollOffset = 0
	m.statusMessage = "Edit mode: Use j/k to navigate, Enter to edit field, :w to save, :wq to save & exit"

	return m
}

func (m Model) saveEdit() Model {
	if m.editType == EditTypeNone {
		m.statusMessage = "Nothing to save"
		return m
	}

	switch m.editType {
	case EditTypeRequest:
		if m.collection == nil {
			m.statusMessage = "Error: No collection loaded"
			return m
		}

		if !m.updateRequestInCollection(m.editItemPath, m.editOriginalName, m.editItemName, m.editRequest) {
			m.statusMessage = "Error: Failed to update request in collection"
			return m
		}

		itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editOriginalName)
		m.modifiedRequests[itemID] = m.editRequest
		m.modifiedItems[itemID] = true
		m.modifiedCollections[m.editCollectionName] = true

		if err := m.parser.SaveCollection(m.editCollectionName); err != nil {
			m.statusMessage = fmt.Sprintf("Failed to save collection: %v", err)
			return m
		}

		m.statusMessage = "Saved changes to collection file"
		delete(m.modifiedItems, itemID)
		if len(m.modifiedItems) == 0 {
			delete(m.modifiedCollections, m.editCollectionName)
		}
	}

	return m
}

func (m Model) saveAllModifiedRequests() Model {
	if len(m.modifiedCollections) == 0 {
		m.statusMessage = "No unsaved changes"
		return m
	}

	savedCount := 0
	var errors []string

	for collectionName := range m.modifiedCollections {
		if err := m.parser.SaveCollection(collectionName); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", collectionName, err))
		} else {
			savedCount++
		}
	}

	if len(errors) > 0 {
		m.statusMessage = fmt.Sprintf("Saved %d collections, %d errors: %s", savedCount, len(errors), strings.Join(errors, "; "))
	} else {
		m.statusMessage = fmt.Sprintf("Saved %d collection(s) to file", savedCount)
		m.modifiedCollections = make(map[string]bool)
		m.modifiedItems = make(map[string]bool)
	}

	return m
}

func (m Model) handleEditModeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if !m.updateRequestInCollection(m.editItemPath, m.editOriginalName, m.editItemName, m.editRequest) {
			m.statusMessage = "Error: Failed to update request in collection"
			return m, nil
		}

		itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editOriginalName)
		m.modifiedRequests[itemID] = m.editRequest
		m.modifiedItems[itemID] = true
		m.modifiedCollections[m.editCollectionName] = true
		m.mode = m.previousMode
		m.editType = EditTypeNone
		m.editFieldMode = false
		m = m.refreshCurrentView()
		m.statusMessage = "Changes saved to memory (use :w to write to file)"
		return m, nil

	case ":":
		m.commandMode = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
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

	case "enter":
		m.editFieldMode = true
		fieldValue := m.getCurrentFieldValue()
		fieldName := []string{"Name", "Method", "URL", "Headers", "Body"}[m.editFieldCursor]

		if m.editFieldCursor >= 3 {
			m.editFieldTextArea.SetValue(fieldValue)
			m.editFieldTextArea.Focus()
			m.statusMessage = fmt.Sprintf("Editing %s... (Ctrl+S to save, Esc to cancel)", fieldName)
			return m, m.editFieldTextArea.Focus()
		} else {
			m.editFieldInput.SetValue(fieldValue)
			m.editFieldInput.Focus()
			m.statusMessage = fmt.Sprintf("Editing %s... (Enter to save, Esc to cancel)", fieldName)
			return m, m.editFieldInput.Focus()
		}
	}

	return m, nil
}

func (m Model) handleFieldEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	isMultiLineField := m.editFieldCursor == 3 || m.editFieldCursor == 4

	switch msg.Type {
	case tea.KeyEsc:
		m.editFieldMode = false
		if isMultiLineField {
			m.editFieldTextArea.Blur()
			m.editFieldTextArea.SetValue("")
		} else {
			m.editFieldInput.Blur()
			m.editFieldInput.SetValue("")
		}
		m.statusMessage = "Field edit cancelled"
		return m, nil

	case tea.KeyCtrlS:
		if isMultiLineField {
			if m.editType == EditTypeRequest && m.editRequest != nil {
				switch m.editFieldCursor {
				case 3:
					m.editRequest.Header = m.parseHeaders(m.editFieldTextArea.Value())
				case 4:
					if m.editRequest.Body == nil {
						m.editRequest.Body = &postman.Body{}
					}
					m.editRequest.Body.Raw = m.editFieldTextArea.Value()
				}
			}
			m.editFieldMode = false
			m.editFieldTextArea.Blur()
			m.statusMessage = "Field updated (use :w to save to file)"
			return m, nil
		}
		return m, nil

	case tea.KeyEnter:
		if !isMultiLineField {
			if m.editType == EditTypeRequest && m.editRequest != nil {
				switch m.editFieldCursor {
				case 0:
					m.editItemName = m.editFieldInput.Value()
				case 1:
					m.editRequest.Method = m.editFieldInput.Value()
				case 2:
					m.editRequest.URL.Raw = m.editFieldInput.Value()
				}
			}
			m.editFieldMode = false
			m.editFieldInput.Blur()
			m.statusMessage = "Field updated (use :w to save to file)"
			return m, nil
		}
		m.editFieldTextArea, cmd = m.editFieldTextArea.Update(msg)
		return m, cmd

	default:
		if isMultiLineField {
			m.editFieldTextArea, cmd = m.editFieldTextArea.Update(msg)
		} else {
			m.editFieldInput, cmd = m.editFieldInput.Update(msg)
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
	if m.editType == EditTypeRequest {
		return 5
	}
	return 0
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

	metrics := m.calculateSplitLayout()
	m.infoViewport.Width = m.width - 8
	m.infoViewport.Height = metrics.popupHeight - 4
	lines := m.buildItemInfoLines()
	content := strings.Join(lines, "\n")
	m.infoViewport.SetContent(content)

	m.statusMessage = "Showing diff (original → modified)"

	return m
}

func (m Model) findOriginalRequest(items []postman.Item, folderPath []string, requestName string) *postman.Request {
	currentItems := items

	for _, folderName := range folderPath {
		found := false
		for _, item := range currentItems {
			if item.IsFolder() && item.Name == folderName {
				currentItems = item.Items
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	for _, item := range currentItems {
		if item.IsRequest() && item.Name == requestName {
			return item.Request
		}
	}

	return nil
}

func (m Model) getCurrentFieldValue() string {
	if m.editType == EditTypeRequest && m.editRequest != nil {
		switch m.editFieldCursor {
		case 0:
			return m.editItemName
		case 1:
			return m.editRequest.Method
		case 2:
			return m.editRequest.URL.Raw
		case 3:
			if len(m.editRequest.Header) > 0 {
				var headerLines []string
				for _, h := range m.editRequest.Header {
					headerLines = append(headerLines, h.Key+": "+h.Value)
				}
				return strings.Join(headerLines, "\n")
			}
			return ""
		case 4:
			if m.editRequest.Body != nil {
				return m.editRequest.Body.Raw
			}
			return ""
		}
	}
	return ""
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

	items := &m.collection.Items

	for i, folderName := range path {
		found := false
		for j := range *items {
			if (*items)[j].Name == folderName && (*items)[j].IsFolder() {
				items = &(*items)[j].Items
				found = true
				break
			}
		}
		if !found {
			return false
		}

		if i == len(path)-1 {
			break
		}
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

	items := &m.collection.Items

	for _, folderName := range m.breadcrumb {
		found := false
		for j := range *items {
			if (*items)[j].Name == folderName && (*items)[j].IsFolder() {
				items = &(*items)[j].Items
				found = true
				break
			}
		}
		if !found {
			m.statusMessage = "Error: Could not find folder in breadcrumb"
			return m
		}
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

	items := &m.collection.Items

	for _, folderName := range m.breadcrumb {
		found := false
		for j := range *items {
			if (*items)[j].Name == folderName && (*items)[j].IsFolder() {
				items = &(*items)[j].Items
				found = true
				break
			}
		}
		if !found {
			m.statusMessage = "Error: Could not find folder in breadcrumb"
			return m
		}
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
				current := m.collection.Items
				for _, crumb := range session.Breadcrumb {
					found := false
					for _, item := range current {
						if item.IsFolder() && item.Name == crumb {
							current = item.Items
							found = true
							break
						}
					}
					if !found {
						m.statusMessage = "Could not restore folder path"
						m = m.loadRequestsList()
						return m
					}
				}

				m.breadcrumb = session.Breadcrumb
				m.currentItems = current
				m.items = []string{}
				for _, item := range current {
					prefix := ""
					if item.IsFolder() {
						prefix = "[DIR] "
					} else if item.IsRequest() {
						prefix = fmt.Sprintf("[%s] ", item.Request.Method)
					} else {
						prefix = "[???] "
					}
					m.items = append(m.items, prefix+item.Name)
				}
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
