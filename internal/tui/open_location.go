package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// openLocationKey reveals the selected collection or environment file.
const openLocationKey = "O"

// openFileLocation reveals path in the platform's file manager: Finder on
// macOS, Explorer on Windows. Other platforms fall back to opening the
// containing directory, since there is no universal "reveal and select" command.
// It is a variable so tests can stub out the actual OS process launch.
var openFileLocation = func(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,"+path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func handleOpenCollectionLocationKey(m Model) (Model, tea.Cmd) {
	if m.cursor >= len(m.items) {
		m.statusMessage = "No collection selected"
		return m, nil
	}
	name := m.items[m.cursor]
	path, exists := m.parser.GetCollectionPath(name)
	if !exists {
		m.statusMessage = fmt.Sprintf("Path not found for collection: %s", name)
		return m, nil
	}
	if err := openFileLocation(path); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to open location: %v", err)
		return m, nil
	}
	m.statusMessage = fmt.Sprintf("Opened location: %s", path)
	return m, nil
}

func handleOpenEnvironmentLocationKey(m Model) (Model, tea.Cmd) {
	if m.cursor >= len(m.items) {
		m.statusMessage = "No environment selected"
		return m, nil
	}
	name := m.items[m.cursor]
	path, exists := m.parser.GetEnvironmentPath(name)
	if !exists {
		m.statusMessage = fmt.Sprintf("Path not found for environment: %s", name)
		return m, nil
	}
	if err := openFileLocation(path); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to open location: %v", err)
		return m, nil
	}
	m.statusMessage = fmt.Sprintf("Opened location: %s", path)
	return m, nil
}
