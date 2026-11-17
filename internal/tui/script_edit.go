package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const (
	preRequestListen  = "prerequest"
	testListen        = "test"
	javascriptType    = "text/javascript"
	scriptFileExt     = ".js"
	scriptTempDirGlob = "postoffice-scripts-*"
	// vimTabPagesFlag opens every file in its own tab page.
	vimTabPagesFlag = "-p"

	preRequestScriptLabel = "pre-request"
	testScriptLabel       = "test"
	preStepScriptLabel    = "pre-step"
	postStepScriptLabel   = "post-step"
)

// scriptListenLabels names a request's script files after the event they belong to.
var scriptListenLabels = map[string]string{
	preRequestListen: preRequestScriptLabel,
	testListen:       testScriptLabel,
}

// scriptBuffer is one script opened in the editor, in its own file.
// EventIndex is the request event it came from, or noScriptEvent when the
// request has no script of that kind yet.
type scriptBuffer struct {
	Label      string
	Listen     string
	EventIndex int
	Content    string
	Path       string
}

const noScriptEvent = -1

type scriptEditTarget int

const (
	scriptTargetRequest scriptEditTarget = iota
	scriptTargetWorkflowStep
)

// VimScriptsEditCompleteMsg is sent when the editor showing every script of
// a request or workflow step exits.
type VimScriptsEditCompleteMsg struct {
	Dir        string
	Buffers    []scriptBuffer
	Target     scriptEditTarget
	FolderPath []string
	ItemName   string
	StepIndex  int
	Err        error
}

func handleEditScriptKey(m Model) (Model, tea.Cmd) {
	if m.mode != ModeRequests || m.cursor >= len(m.currentItems) {
		m.statusMessage = "No request selected"
		return m, nil
	}
	item := m.currentItems[m.cursor]
	if !item.IsRequest() {
		m.statusMessage = "Can only edit scripts for requests, not folders"
		return m, nil
	}
	return m.openRequestScripts(item, m.folderPathForCursor())
}

func handleEditScriptCommand(m Model, _ []string) (Model, tea.Cmd) {
	return handleEditScriptKey(m)
}

// requestScriptBuffers is one buffer per script event of item, plus an empty
// one for a pre-request or test script it doesn't have yet.
func requestScriptBuffers(item postman.Item) []scriptBuffer {
	var buffers []scriptBuffer
	seen := make(map[string]int)
	for i, event := range item.Events {
		label, isScript := scriptListenLabels[event.Listen]
		if !isScript {
			continue
		}
		seen[event.Listen]++
		if seen[event.Listen] > 1 {
			label = fmt.Sprintf("%s-%d", label, seen[event.Listen])
		}
		buffers = append(buffers, scriptBuffer{Label: label, Listen: event.Listen, EventIndex: i, Content: strings.Join(event.Script.Exec, "\n")})
	}
	for _, listen := range []string{preRequestListen, testListen} {
		if seen[listen] == 0 {
			buffers = append(buffers, scriptBuffer{Label: scriptListenLabels[listen], Listen: listen, EventIndex: noScriptEvent})
		}
	}
	return buffers
}

func (m Model) openRequestScripts(item postman.Item, folderPath []string) (Model, tea.Cmd) {
	msg := VimScriptsEditCompleteMsg{
		Buffers:    requestScriptBuffers(item),
		Target:     scriptTargetRequest,
		FolderPath: append([]string{}, folderPath...),
		ItemName:   item.Name,
	}
	return m.openScriptBuffers(msg)
}

func (m Model) openWorkflowStepScripts() (Model, tea.Cmd) {
	if m.activeWorkflow == nil || m.workflowStepCursor >= len(m.activeWorkflow.Steps) {
		m.statusMessage = "No step selected"
		return m, nil
	}
	step := m.activeWorkflow.Steps[m.workflowStepCursor]
	msg := VimScriptsEditCompleteMsg{
		Buffers: []scriptBuffer{
			{Label: preStepScriptLabel, Content: step.PreScript},
			{Label: postStepScriptLabel, Content: step.PostScript},
		},
		Target:    scriptTargetWorkflowStep,
		ItemName:  step.ID,
		StepIndex: m.workflowStepCursor,
	}
	return m.openScriptBuffers(msg)
}

// openScriptBuffers opens every buffer in one editor session, as tab pages
// in vim-family editors.
func (m Model) openScriptBuffers(msg VimScriptsEditCompleteMsg) (Model, tea.Cmd) {
	msg, err := writeScriptBuffers(msg)
	if err != nil {
		m.statusMessage = err.Error()
		return m, nil
	}

	editor := editorBin()
	var args []string
	if vimFamilyEditors[filepath.Base(editor)] {
		args = append(args, vimTabPagesFlag)
	}
	for _, buffer := range msg.Buffers {
		args = append(args, buffer.Path)
	}
	return m, tea.ExecProcess(exec.Command(editor, args...), func(err error) tea.Msg {
		msg.Err = err
		return msg
	})
}

// writeScriptBuffers writes each buffer to its own file, named after its
// label, in a new temp directory that applyVimScriptsEdit removes.
func writeScriptBuffers(msg VimScriptsEditCompleteMsg) (VimScriptsEditCompleteMsg, error) {
	dir, err := os.MkdirTemp("", scriptTempDirGlob)
	if err != nil {
		return msg, fmt.Errorf("failed to create script directory: %w", err)
	}
	for i := range msg.Buffers {
		buffer := &msg.Buffers[i]
		buffer.Path = filepath.Join(dir, buffer.Label+scriptFileExt)
		if err := os.WriteFile(buffer.Path, []byte(buffer.Content), 0o600); err != nil {
			os.RemoveAll(dir)
			return msg, fmt.Errorf("failed to write %s script: %w", buffer.Label, err)
		}
	}
	msg.Dir = dir
	return msg, nil
}

// editedScripts reads every buffer back, returning the changed ones with
// their new content.
func editedScripts(buffers []scriptBuffer) ([]scriptBuffer, error) {
	var changed []scriptBuffer
	for _, buffer := range buffers {
		data, isChanged, err := readEditedFile(buffer.Path, []byte(buffer.Content))
		if err != nil {
			return nil, err
		}
		if isChanged {
			buffer.Content = strings.TrimRight(string(data), "\n")
			changed = append(changed, buffer)
		}
	}
	return changed, nil
}

func scriptLabels(buffers []scriptBuffer) string {
	labels := make([]string, len(buffers))
	for i, buffer := range buffers {
		labels[i] = buffer.Label
	}
	return strings.Join(labels, ", ")
}

func (m Model) applyVimScriptsEdit(msg VimScriptsEditCompleteMsg) Model {
	defer os.RemoveAll(msg.Dir)

	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}
	changed, err := editedScripts(msg.Buffers)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to read edited script: %v", err)
		return m
	}
	if len(changed) == 0 {
		m.statusMessage = noEditorChangesMessage
		return m
	}

	if msg.Target == scriptTargetWorkflowStep {
		return m.saveWorkflowStepScripts(msg, changed)
	}
	return m.saveRequestScripts(msg, changed)
}

func (m Model) saveRequestScripts(msg VimScriptsEditCompleteMsg, changed []scriptBuffer) Model {
	item := m.findItemByPath(msg.FolderPath, msg.ItemName)
	if item == nil {
		m.statusMessage = fmt.Sprintf("Could not find request %q to save its scripts", msg.ItemName)
		m.statusIsError = true
		return m
	}

	for _, buffer := range changed {
		script := postman.Script{Type: javascriptType, Exec: splitLines(buffer.Content)}
		if buffer.Content == "" {
			script.Exec = []string{}
		}
		if buffer.EventIndex == noScriptEvent {
			if buffer.Content != "" {
				item.Events = append(item.Events, postman.Event{Listen: buffer.Listen, Script: script})
			}
			continue
		}
		item.Events[buffer.EventIndex].Script.Type = script.Type
		item.Events[buffer.EventIndex].Script.Exec = script.Exec
	}

	if err := m.saveOwningCollection(msg.FolderPath); err != nil {
		m.markCollectionModified(msg.FolderPath)
		m.statusMessage = fmt.Sprintf("Updated %s scripts but failed to save: %v", scriptLabels(changed), err)
		m.statusIsError = true
		return m
	}
	m = m.refreshCurrentView()
	m.statusMessage = fmt.Sprintf("Saved %s script(s) for '%s'", scriptLabels(changed), msg.ItemName)
	return m
}

func (m Model) saveWorkflowStepScripts(msg VimScriptsEditCompleteMsg, changed []scriptBuffer) Model {
	if m.activeWorkflow == nil || msg.StepIndex >= len(m.activeWorkflow.Steps) {
		m.statusMessage = fmt.Sprintf("Could not find step %q to save its scripts", msg.ItemName)
		m.statusIsError = true
		return m
	}

	step := &m.activeWorkflow.Steps[msg.StepIndex]
	for _, buffer := range changed {
		switch buffer.Label {
		case preStepScriptLabel:
			step.PreScript = buffer.Content
		case postStepScriptLabel:
			step.PostScript = buffer.Content
		}
	}

	var err error
	if m, err = m.saveActiveWorkflow(); err != nil {
		m.statusMessage = fmt.Sprintf("Failed to save workflow: %v", err)
		m.statusIsError = true
		return m
	}
	m.statusMessage = fmt.Sprintf("Saved %s script(s) for step %q", scriptLabels(changed), step.ID)
	return m
}
