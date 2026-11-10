package tui

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/logger"
)

const (
	undoKey = "u"
	redoKey = "ctrl+r"
	// maxUndoSteps caps the undo history, dropping the oldest steps.
	maxUndoSteps = 50
	// restoredFileMode is used when a restored file's mode can't be read.
	restoredFileMode = 0o644
)

type savedFileKind int

const (
	savedCollection savedFileKind = iota
	savedEnvironment
)

func (k savedFileKind) String() string {
	if k == savedEnvironment {
		return "environment"
	}
	return "collection"
}

// fileSnapshot is a collection or environment file's contents before (or,
// on the redo stack, after) one edit the TUI saved.
type fileSnapshot struct {
	kind savedFileKind
	name string
	path string
	data []byte
}

// undoHistory undoes edits by restoring the files they were saved to. It is
// shared by every copy of the Model, like the parser.
type undoHistory struct {
	undo []fileSnapshot
	redo []fileSnapshot
}

func newUndoHistory() *undoHistory {
	return &undoHistory{}
}

// recordBeforeSave keeps the file's current contents so the save about to
// overwrite them can be undone. A new edit can't be redone past.
func (h *undoHistory) recordBeforeSave(kind savedFileKind, name, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		logger.LogError("recordBeforeSave", path, err)
		return
	}
	h.undo = pushSnapshot(h.undo, fileSnapshot{kind: kind, name: name, path: path, data: data})
	h.redo = nil
}

func pushSnapshot(stack []fileSnapshot, snapshot fileSnapshot) []fileSnapshot {
	stack = append(stack, snapshot)
	if len(stack) > maxUndoSteps {
		stack = stack[len(stack)-maxUndoSteps:]
	}
	return stack
}

// saveCollectionUndoable saves a collection the user edited, so it can be undone.
func (m Model) saveCollectionUndoable(name string) error {
	if path, exists := m.parser.GetCollectionPath(name); exists {
		m.undoHistory.recordBeforeSave(savedCollection, name, path)
	}
	return m.parser.SaveCollection(name)
}

// saveEnvironmentUndoable saves an environment the user edited, so it can be undone.
func (m Model) saveEnvironmentUndoable(name string) error {
	if path, exists := m.parser.GetEnvironmentPath(name); exists {
		m.undoHistory.recordBeforeSave(savedEnvironment, name, path)
	}
	return m.parser.SaveEnvironment(name)
}

func handleUndoKey(m Model) (Model, tea.Cmd) {
	return m.restoreSnapshot(&m.undoHistory.undo, &m.undoHistory.redo, "Undid", "Nothing to undo"), nil
}

func handleRedoKey(m Model) (Model, tea.Cmd) {
	return m.restoreSnapshot(&m.undoHistory.redo, &m.undoHistory.undo, "Redid", "Nothing to redo"), nil
}

// restoreSnapshot writes the newest snapshot on from back to its file and
// reloads it, keeping the file's current contents on to so the step can be
// reversed. It refuses while the file has unsaved edits, which reloading
// would lose.
func (m Model) restoreSnapshot(from, to *[]fileSnapshot, verb, emptyMessage string) Model {
	if len(*from) == 0 {
		m.statusMessage = emptyMessage
		return m
	}
	snapshot := (*from)[len(*from)-1]
	if m.hasUnsavedEdits(snapshot) {
		m.statusMessage = fmt.Sprintf("Save or discard the unsaved changes to %s first (:w or :changes)", snapshot.name)
		m.statusIsError = true
		return m
	}

	current, err := os.ReadFile(snapshot.path)
	if err == nil {
		err = writeFileKeepingMode(snapshot.path, snapshot.data)
	}
	if err == nil {
		err = m.reloadSnapshotFile(snapshot)
	}
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to restore %s: %v", snapshot.name, err)
		m.statusIsError = true
		return m
	}

	*from = (*from)[:len(*from)-1]
	*to = pushSnapshot(*to, fileSnapshot{kind: snapshot.kind, name: snapshot.name, path: snapshot.path, data: current})
	m = m.refreshAfterRestore(snapshot)
	m.statusMessage = fmt.Sprintf("%s last change to %s %s", verb, snapshot.kind, snapshot.name)
	return m
}

func (m Model) hasUnsavedEdits(snapshot fileSnapshot) bool {
	if snapshot.kind == savedEnvironment {
		return m.modifiedEnvironments[snapshot.name]
	}
	return m.isCollectionUnsaved(snapshot.name)
}

func (m Model) reloadSnapshotFile(snapshot fileSnapshot) error {
	if snapshot.kind == savedEnvironment {
		_, err := m.parser.LoadEnvironment(snapshot.path)
		return err
	}
	_, err := m.parser.LoadCollection(snapshot.path)
	return err
}

// refreshAfterRestore points the views at the reloaded file.
func (m Model) refreshAfterRestore(snapshot fileSnapshot) Model {
	if snapshot.kind == savedEnvironment {
		if m.environment != nil {
			m = m.reloadActiveEnvironment()
		}
		return m
	}

	if m.isViewingMergedCollection() {
		m = m.reloadMergedCollection()
	} else if m.collection != nil && m.collection.Info.Name == snapshot.name {
		m.collection, _ = m.parser.GetCollection(snapshot.name)
	}
	if m.mode == ModeRequests && m.collection != nil {
		cursor := m.cursor
		m = m.openRequestsFolder(m.breadcrumb)
		m.cursor = min(cursor, max(len(m.currentItems)-1, 0))
	}
	return m
}

// writeFileKeepingMode overwrites path (or the file it links to) with data.
func writeFileKeepingMode(path string, data []byte) error {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	mode := os.FileMode(restoredFileMode)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	logger.LogFileWrite(path)
	return os.WriteFile(path, data, mode)
}
