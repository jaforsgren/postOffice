package tui

import (
	"fmt"
	"strings"
	"postOffice/internal/postman"

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
		m = m.executeCommand()
		m.commandMode = false
		m.commandInput = ""
		return m, nil

	case tea.KeyBackspace:
		if len(m.commandInput) > 0 {
			m.commandInput = m.commandInput[:len(m.commandInput)-1]
		}
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

	case "backspace", "h":
		if len(m.breadcrumb) > 0 {
			m = m.navigateUp()
		}
		return m, nil
	}

	return m, nil
}

func (m Model) executeCommand() Model {
	cmd := strings.TrimSpace(m.commandInput)
	parts := strings.Fields(cmd)

	if len(parts) == 0 {
		return m
	}

	switch parts[0] {
	case "collections", "c":
		m.mode = ModeCollections
		m.loadCollectionsList()
		m.statusMessage = "Showing collections"

	case "request", "r":
		if m.collection != nil {
			m.mode = ModeRequests
			m.loadRequestsList()
			m.statusMessage = "Showing requests"
		} else {
			m.statusMessage = "No collection loaded. Load a collection first."
		}

	case "load", "l":
		if len(parts) > 1 {
			m = m.loadCollection(parts[1])
		} else {
			m.statusMessage = "Usage: load <path>"
		}

	case "quit", "q":
		return m

	default:
		m.statusMessage = fmt.Sprintf("Unknown command: %s", parts[0])
	}

	return m
}

func (m Model) loadCollectionsList() {
	collections := m.parser.ListCollections()
	m.items = collections
	m.cursor = 0
	m.breadcrumb = []string{}
}

func (m Model) loadRequestsList() {
	if m.collection == nil {
		m.items = []string{}
		return
	}

	m.items = []string{}
	m.currentItems = m.collection.Items
	for _, item := range m.collection.Items {
		prefix := ""
		if item.IsFolder() {
			prefix = "[DIR] "
		} else if item.IsRequest() {
			prefix = fmt.Sprintf("[%s] ", item.Request.Method)
		}
		m.items = append(m.items, prefix+item.Name)
	}
	m.cursor = 0
}

func (m Model) loadCollection(path string) Model {
	collection, err := m.parser.LoadCollection(path)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to load collection: %v", err)
		return m
	}

	m.collection = collection
	m.statusMessage = fmt.Sprintf("Loaded collection: %s", collection.Info.Name)
	m.mode = ModeRequests
	m.loadRequestsList()
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
		m.loadRequestsList()
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
			}
			m.items = append(m.items, prefix+item.Name)
		}
		m.cursor = 0
	}

	return m
}
