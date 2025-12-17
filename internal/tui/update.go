package tui

import (
	"fmt"
	"postOffice/internal/postman"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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
		return m.handleNormalMode(msg)
	}

	return m, nil
}

func (m Model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.commandMode = false
		m.commandInput = ""
		m.commandSuggestion = ""
		m.historyIndex = -1
		return m, nil

	case tea.KeyEnter:
		var cmd tea.Cmd
		input := strings.TrimSpace(m.commandInput)
		if input != "" {
			if len(m.commandHistory) == 0 || m.commandHistory[len(m.commandHistory)-1] != input {
				m.commandHistory = append(m.commandHistory, input)
			}
		}
		m, cmd = m.executeCommand()
		m.commandMode = false
		m.commandInput = ""
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
			m.commandInput = m.commandHistory[m.historyIndex]
			m.commandSuggestion = ""
		}
		return m, nil

	case tea.KeyDown:
		if m.historyIndex >= 0 {
			if m.historyIndex < len(m.commandHistory)-1 {
				m.historyIndex++
				m.commandInput = m.commandHistory[m.historyIndex]
			} else {
				m.historyIndex = -1
				m.commandInput = ""
			}
			m.commandSuggestion = ""
		}
		return m, nil

	case tea.KeyTab:
		if m.commandSuggestion != "" {
			m.commandInput = m.commandSuggestion
			m.commandSuggestion = ""
		}
		return m, nil

	case tea.KeyBackspace:
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
			m.commandSuggestion = m.getCommandSuggestion()
		}
		return m, nil

	case tea.KeySpace:
		m.commandInput += " "
		m.commandSuggestion = ""
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			m.commandInput += string(msg.Runes)
			m.commandSuggestion = m.getCommandSuggestion()
		}
		return m, nil
	}
}

func (m Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchMode = false
		m.searchQuery = ""
		m.searchActive = false
		m.items = m.allItems
		m.currentItems = m.allCurrentItems
		m.cursor = 0
		m.statusMessage = "Search cancelled"
		return m, nil

	case tea.KeyEnter:
		m.searchMode = false
		m.searchActive = len(m.items) > 0 && m.searchQuery != ""
		if len(m.items) > 0 {
			m.statusMessage = fmt.Sprintf("Found %d results (press Esc to clear search)", len(m.items))
		} else {
			m.statusMessage = "No results found"
		}
		return m, nil

	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m = m.filterItems()
		}
		return m, nil

	case tea.KeySpace:
		m.searchQuery += " "
		m = m.filterItems()
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			m.searchQuery += string(msg.Runes)
			m = m.filterItems()
		}
		return m, nil
	}
}

func (m Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == ":" {
		m.commandMode = true
		m.commandInput = ""
		return m, nil
	}

	newModel, cmd, handled := m.commandRegistry.HandleKey(m, key)
	if handled {
		return newModel, cmd
	}

	return m, nil
}

func (m Model) executeCommand() (Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.commandInput)
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
	input := strings.TrimSpace(m.commandInput)
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
				m.lastResponse = m.executor.Execute(requestToExecute, variables)
				m.scrollOffset = 0
				m.mode = ModeResponse
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
	m.searchQuery = ""
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
	m.searchQuery = ""
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
	if m.searchQuery == "" {
		m.items = m.allItems
		m.currentItems = m.allCurrentItems
		m.filteredItems = []string{}
		m.filteredIndices = []int{}
		m.cursor = 0
		return m
	}

	query := strings.ToLower(m.searchQuery)

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
			displayItems, foundItems, indices := m.searchItemsRecursive(m.collection.Items, m.searchQuery, "")
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

func (m Model) handleEditModeKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = m.previousMode
		m.editType = EditTypeNone
		m.editFieldMode = false
		m.statusMessage = "Edit cancelled"
		return m, nil

	case ":":
		m.commandMode = true
		m.commandInput = ""
		return m, nil

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
		m.editFieldInput = m.getCurrentFieldValue()
		m.statusMessage = "Editing field... (Enter to confirm, Esc to cancel)"
		return m, nil
	}

	return m, nil
}

func (m Model) handleFieldEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.editFieldMode = false
		m.editFieldInput = ""
		m.statusMessage = "Field edit cancelled"
		return m, nil

	case tea.KeyEnter:
		m.setCurrentFieldValue(m.editFieldInput)
		m.editFieldMode = false
		m.editFieldInput = ""
		m.statusMessage = "Field updated (use :w to save)"
		return m, nil

	case tea.KeyBackspace:
		if len(m.editFieldInput) > 0 {
			m.editFieldInput = m.editFieldInput[:len(m.editFieldInput)-1]
		}
		return m, nil

	case tea.KeySpace:
		m.editFieldInput += " "
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			m.editFieldInput += string(msg.Runes)
		}
		return m, nil
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
		return 4
	}
	return 0
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
			if m.editRequest.Body != nil {
				return m.editRequest.Body.Raw
			}
			return ""
		}
	}
	return ""
}

func (m Model) setCurrentFieldValue(value string) {
	if m.editType == EditTypeRequest && m.editRequest != nil {
		switch m.editFieldCursor {
		case 0:
			m.editItemName = value
		case 1:
			m.editRequest.Method = value
		case 2:
			m.editRequest.URL.Raw = value
		case 3:
			if m.editRequest.Body == nil {
				m.editRequest.Body = &postman.Body{}
			}
			m.editRequest.Body.Raw = value
		}
	}
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
