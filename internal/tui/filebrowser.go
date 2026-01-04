package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func listDirectory(path string) ([]os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	entries, err := file.Readdir(-1)
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	return entries, nil
}

func filterFileEntries(entries []os.FileInfo) []os.FileInfo {
	var filtered []os.FileInfo
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		if entry.IsDir() {
			filtered = append(filtered, entry)
			continue
		}

		if strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func (m Model) enterFileBrowser(command string) Model {
	m.fileBrowserActive = true
	m.fileBrowserCommand = command
	m.fileBrowserPath = []string{}
	m.previousMode = m.mode
	m.mode = ModeFileBrowser
	m = m.loadDirectoryContents()
	return m
}

func (m Model) loadDirectoryContents() Model {
	entries, err := listDirectory(m.fileBrowserCwd)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Error reading directory: %v", err)
		return m
	}

	filtered := filterFileEntries(entries)

	m.items = make([]string, len(filtered))
	for i, entry := range filtered {
		if entry.IsDir() {
			m.items[i] = "[DIR] " + entry.Name()
		} else {
			m.items[i] = "[JSON] " + entry.Name()
		}
	}

	m.cursor = 0
	m.currentItems = []postman.Item{}

	if len(m.items) == 0 {
		m.statusMessage = "No .json files or directories found"
	} else {
		m.statusMessage = fmt.Sprintf("%d items", len(m.items))
	}

	return m
}

func (m Model) navigateIntoDirectory(dirName string) Model {
	newPath := filepath.Join(m.fileBrowserCwd, dirName)

	fileInfo, err := os.Stat(newPath)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Error accessing directory: %v", err)
		return m
	}

	if !fileInfo.IsDir() {
		m.statusMessage = "Not a directory"
		return m
	}

	m.fileBrowserCwd = newPath
	m.fileBrowserPath = append(m.fileBrowserPath, dirName)
	return m.loadDirectoryContents()
}

func (m Model) navigateUpDirectory() Model {
	if len(m.fileBrowserPath) == 0 {
		parent := filepath.Dir(m.fileBrowserCwd)
		if parent == m.fileBrowserCwd {
			m.statusMessage = "Already at root directory"
			return m
		}
		m.fileBrowserCwd = parent
	} else {
		m.fileBrowserPath = m.fileBrowserPath[:len(m.fileBrowserPath)-1]
		m.fileBrowserCwd = filepath.Dir(m.fileBrowserCwd)
	}

	return m.loadDirectoryContents()
}

func (m Model) selectFileEntry() Model {
	if len(m.items) == 0 {
		return m
	}

	selected := m.items[m.cursor]

	if strings.HasPrefix(selected, "[DIR]") {
		dirName := strings.TrimPrefix(selected, "[DIR] ")
		return m.navigateIntoDirectory(dirName)
	}

	if strings.HasPrefix(selected, "[JSON]") {
		fileName := strings.TrimPrefix(selected, "[JSON] ")
		fullPath := filepath.Join(m.fileBrowserCwd, fileName)

		m.fileBrowserActive = false
		m.mode = m.previousMode

		if m.fileBrowserCommand == "load" {
			m = m.loadCollection(fullPath)
		} else if m.fileBrowserCommand == "loadenv" {
			m = m.loadEnvironment(fullPath)
		}
	}

	return m
}

func (m Model) exitFileBrowser() Model {
	m.fileBrowserActive = false
	m.mode = m.previousMode
	m.fileBrowserCwd = ""
	m.fileBrowserPath = []string{}
	m.statusMessage = "File browser cancelled"
	return m
}

func (m Model) handleFileBrowserKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc", "q":
		return m.exitFileBrowser(), nil

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
		return m.selectFileEntry(), nil

	case "h", "backspace":
		return m.navigateUpDirectory(), nil

	case ":":
		m.commandMode = true
		m.commandInput.SetValue("")
		m.commandInput.Focus()
		return m, textinput.Blink
	}

	return m, nil
}

func (m Model) renderFileBrowser() string {
	var lines []string

	pathDisplay := "File Browser: " + m.fileBrowserCwd
	lines = append(lines, folderStyle.Render(pathDisplay))
	lines = append(lines, "")

	if len(m.fileBrowserPath) > 0 {
		breadcrumbLine := "/ " + strings.Join(m.fileBrowserPath, " / ")
		lines = append(lines, folderStyle.Render(breadcrumbLine))
		lines = append(lines, "")
	}

	contentHeight := m.height - 10
	visibleLines := contentHeight - len(lines)

	startIdx := 0
	endIdx := len(m.items)

	if len(m.items) > visibleLines {
		if m.cursor < visibleLines/2 {
			startIdx = 0
			endIdx = visibleLines
		} else if m.cursor > len(m.items)-visibleLines/2 {
			startIdx = len(m.items) - visibleLines
			endIdx = len(m.items)
		} else {
			startIdx = m.cursor - visibleLines/2
			endIdx = m.cursor + visibleLines/2
		}
	}

	for i := startIdx; i < endIdx && i < len(m.items); i++ {
		line := m.items[i]
		if i == m.cursor {
			line = "> " + line
			lines = append(lines, selectedItemStyle.Render(line))
		} else {
			line = "  " + line
			style := normalItemStyle
			if strings.HasPrefix(m.items[i], "[DIR]") {
				style = folderStyle
			}
			lines = append(lines, style.Render(line))
		}
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(contentHeight).
		Width(m.width - 4).
		Render(content)
}
