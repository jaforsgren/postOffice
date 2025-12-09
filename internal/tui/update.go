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
		return m.handleNormalMode(msg)
	}

	return m, nil
}

func (m Model) handleCommandMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.commandMode = false
		m.commandInput = ""
		return m, nil

	case tea.KeyEnter:
		var cmd tea.Cmd
		m, cmd = m.executeCommand()
		m.commandMode = false
		m.commandInput = ""
		return m, cmd

	case tea.KeyBackspace:
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
		return m, nil

	case tea.KeySpace:
		m.commandInput += " "
		return m, nil

	default:
		if msg.Type == tea.KeyRunes {
			m.commandInput += string(msg.Runes)
		}
		return m, nil
	}
}

func (m Model) handleSearchMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchMode = false
		m.searchQuery = ""
		m.items = m.allItems
		m.currentItems = m.allCurrentItems
		m.cursor = 0
		m.statusMessage = "Search cancelled"
		return m, nil

	case tea.KeyEnter:
		m.searchMode = false
		if len(m.items) > 0 {
			m.statusMessage = fmt.Sprintf("Found %d results", len(m.items))
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
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case ":":
		m.commandMode = true
		m.commandInput = ""
		return m, nil

	case "/":
		if m.mode == ModeCollections || m.mode == ModeRequests || m.mode == ModeEnvironments {
			m.searchMode = true
			m.searchQuery = ""
			m.allItems = m.items
			m.allCurrentItems = m.currentItems
			m.statusMessage = "Enter search query (Esc to cancel, Enter to confirm)"
		}
		return m, nil

	case "up", "k":
		if m.mode == ModeInfo || m.mode == ModeResponse {
			if m.scrollOffset > 0 {
				m.scrollOffset--
			}
		} else {
			if m.cursor > 0 {
				m.cursor--
			}
		}
		return m, nil

	case "down", "j":
		if m.mode == ModeInfo || m.mode == ModeResponse {
			m.scrollOffset++
		} else {
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		}
		return m, nil

	case "enter":
		return m.handleSelection(), nil

	case "i":
		if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
			m.currentInfoItem = &m.currentItems[m.cursor]
			m.scrollOffset = 0
			m.previousMode = m.mode
			m.mode = ModeInfo
			m.statusMessage = "Showing item info"
		} else if m.mode == ModeEnvironments && m.environment != nil {
			m.scrollOffset = 0
			m.previousMode = m.mode
			m.mode = ModeInfo
			m.statusMessage = "Showing environment info"
		}
		return m, nil

	case "esc", "left", "backspace", "h":
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
		if len(m.breadcrumb) > 0 {
			m = m.navigateUp()
		}
		return m, nil
	}

	return m, nil
}

func (m Model) executeCommand() (Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.commandInput)
	parts := strings.Fields(cmd)

	if len(parts) == 0 {
		return m, nil
	}

	switch parts[0] {
	case "collections", "c", "col":
		m.mode = ModeCollections
		m = m.loadCollectionsList()
		m.statusMessage = "Showing collections"

	case "request", "requests", "r", "req":
		if m.collection != nil {
			m.mode = ModeRequests
			m = m.loadRequestsList()
			m.statusMessage = "Showing requests"
		} else {
			m.statusMessage = "No collection loaded. Load a collection first."
		}

	case "load", "l":
		if len(parts) > 1 {
			pathStart := strings.Index(cmd, parts[0]) + len(parts[0])
			path := strings.TrimSpace(cmd[pathStart:])
			m = m.loadCollection(path)
		} else {
			m.statusMessage = "Usage: load <path>"
		}

	case "loadenv", "le", "env-load":
		if len(parts) > 1 {
			pathStart := strings.Index(cmd, parts[0]) + len(parts[0])
			path := strings.TrimSpace(cmd[pathStart:])
			m = m.loadEnvironment(path)
		} else {
			m.statusMessage = "Usage: loadenv <path>"
		}

	case "environments", "e", "env", "envs":
		m.mode = ModeEnvironments
		m = m.loadEnvironmentsList()
		m.statusMessage = "Showing environments"

	case "quit", "q", "exit":
		return m, tea.Quit

	case "help", "h", "?":
		m.statusMessage = "Commands: :load/:l <path> | :loadenv/:le <path> | :collections/:c | :environments/:e | :requests/:r | /search | :info/:i | :quit/:q"

	case "debug", "d":
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

	case "info", "i":
		if m.mode == ModeRequests && len(m.currentItems) > 0 && m.cursor < len(m.currentItems) {
			m.currentInfoItem = &m.currentItems[m.cursor]
			m.scrollOffset = 0
			m.mode = ModeInfo
			m.statusMessage = "Showing item info"
		} else if m.mode == ModeCollections {
			m.statusMessage = "Info command is only available in requests mode"
		} else {
			m.statusMessage = "No item selected"
		}

	default:
		m.statusMessage = fmt.Sprintf("Unknown command: %s (try :help)", parts[0])
	}

	return m, nil
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
				m.statusMessage = fmt.Sprintf("Executing: %s %s", item.Request.Method, item.Name)
				m.lastResponse = m.executor.Execute(item.Request)
				m.scrollOffset = 0
				m.mode = ModeResponse
				if m.lastResponse.Error != nil {
					m.statusMessage = fmt.Sprintf("Request failed: %v", m.lastResponse.Error)
				} else {
					m.statusMessage = fmt.Sprintf("Response: %s (%v)", m.lastResponse.Status, m.lastResponse.Duration)
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
