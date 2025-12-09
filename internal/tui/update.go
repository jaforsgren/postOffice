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

func (m Model) handleNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case ":":
		m.commandMode = true
		m.commandInput = ""
		return m, nil

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil

	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return m, nil

	case "enter":
		return m.handleSelection(), nil

	case "esc", "left", "backspace", "h":
		if m.mode == ModeResponse {
			m.mode = ModeRequests
			m.statusMessage = "Closed response view"
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

	case "quit", "q", "exit":
		return m, tea.Quit

	case "help", "h", "?":
		m.statusMessage = "Commands: :load <path> | :collections (c) | :requests (r) | :quit (q)"

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
	case ModeRequests:
		if m.cursor < len(m.currentItems) {
			item := m.currentItems[m.cursor]
			if item.IsFolder() {
				m = m.navigateInto(item)
			} else if item.IsRequest() {
				m.statusMessage = fmt.Sprintf("Executing: %s %s", item.Request.Method, item.Name)
				m.lastResponse = m.executor.Execute(item.Request)
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
