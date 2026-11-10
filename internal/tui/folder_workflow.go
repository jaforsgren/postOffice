package tui

import (
	"errors"
	"fmt"
	"slices"

	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

// folderWorkflowIndicator marks a folder in the requests list that has a saved workflow.
// It renders two cells wide; formatItemLine pads by display width to keep columns aligned.
const folderWorkflowIndicator = "⚡"

const (
	confirmFolderRunKey = "y"
	cancelFolderRunKey  = "n"
)

var errCollectionPathNotFound = errors.New("collection path not found")

// pendingFolderRun is a folder the user asked to run as a workflow, awaiting confirmation.
type pendingFolderRun struct {
	folder     postman.Item
	folderPath []string // full path including the folder's own name
}

func folderRunPrompt(folderPath []string) string {
	return fmt.Sprintf("Run folder '%s' as workflow? (%s/%s)",
		workflow.FolderWorkflowName(folderPath), confirmFolderRunKey, cancelFolderRunKey)
}

// requestFolderRun asks for confirmation before running the folder at the cursor as a workflow.
func (m Model) requestFolderRun(folder postman.Item, parentPath []string) Model {
	if m.workflowChan != nil {
		m.statusMessage = "Workflow already running"
		return m
	}
	m.pendingFolderRun = &pendingFolderRun{
		folder:     folder,
		folderPath: append(slices.Clone(parentPath), folder.Name),
	}
	m.statusMessage = folderRunPrompt(m.pendingFolderRun.folderPath)
	return m
}

// handleFolderRunConfirmKeys answers the pending folder-run prompt: the confirm key runs the
// folder, any other key cancels, so the prompt can never trap the user.
func (m Model) handleFolderRunConfirmKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	run := *m.pendingFolderRun
	m.pendingFolderRun = nil
	if msg.String() != confirmFolderRunKey {
		m.statusMessage = fmt.Sprintf("Cancelled running folder '%s'", workflow.FolderWorkflowName(run.folderPath))
		return m, nil
	}
	return m.runFolderAsWorkflow(run)
}

// runFolderAsWorkflow runs the folder's saved workflow, generating and saving one first when the
// folder has none yet (or its requests changed and the saved one was never hand-edited).
func (m Model) runFolderAsWorkflow(run pendingFolderRun) (Model, tea.Cmd) {
	if m.collection == nil {
		m.statusMessage = "Load a collection first"
		return m, nil
	}

	generated, err := workflow.NewFolderWorkflow(run.folder, run.folderPath)
	if err != nil {
		return m.folderRunError(err), nil
	}

	collectionPath, saved, err := m.discoverCollectionWorkflows()
	if err != nil {
		return m.folderRunError(err), nil
	}

	wf, resolution := workflow.ResolveFolderWorkflow(findWorkflowByID(saved, generated.ID), generated)
	if resolution.NeedsSave() {
		if err := workflow.SaveWorkflow(wf, workflow.WorkflowPath(collectionPath, wf.ID)); err != nil {
			return m.folderRunError(fmt.Errorf("failed to save workflow: %w", err)), nil
		}
		if _, saved, err = m.discoverCollectionWorkflows(); err != nil {
			return m.folderRunError(err), nil
		}
	}
	m.workflows = saved
	m = m.cacheSavedWorkflowIDs(saved)

	m, cmd := m.startWorkflow(wf)
	if cmd != nil {
		m.statusMessage = folderRunStatus(wf, resolution)
	}
	return m, cmd
}

func (m Model) folderRunError(err error) Model {
	m.statusMessage = fmt.Sprintf("Cannot run folder as workflow: %v", err)
	m.statusIsError = true
	return m
}

func folderRunStatus(wf *workflow.Workflow, resolution workflow.FolderWorkflowResolution) string {
	switch resolution {
	case workflow.FolderWorkflowCreated:
		return fmt.Sprintf("Saved workflow %q, running %d step(s)", wf.ID, len(wf.Steps))
	case workflow.FolderWorkflowRegenerated:
		return fmt.Sprintf("Folder changed: regenerated workflow %q, running %d step(s)", wf.ID, len(wf.Steps))
	case workflow.FolderWorkflowCustomizedStale:
		return fmt.Sprintf("Folder changed but workflow %q was edited: running it unchanged (delete it to regenerate)", wf.ID)
	default:
		return fmt.Sprintf("Running saved workflow %q", wf.ID)
	}
}

// discoverCollectionWorkflows loads every workflow saved next to the current collection file.
func (m Model) discoverCollectionWorkflows() (string, []*workflow.Workflow, error) {
	collectionPath, exists := m.parser.GetCollectionPath(m.collection.Info.Name)
	if !exists {
		return "", nil, errCollectionPathNotFound
	}
	wfs, err := workflow.DiscoverWorkflows(workflow.WorkflowsDir(collectionPath))
	return collectionPath, wfs, err
}

func findWorkflowByID(wfs []*workflow.Workflow, id string) *workflow.Workflow {
	for _, wf := range wfs {
		if wf.ID == id {
			return wf
		}
	}
	return nil
}

func (m Model) cacheSavedWorkflowIDs(wfs []*workflow.Workflow) Model {
	m.savedWorkflowIDs = make(map[string]bool, len(wfs))
	for _, wf := range wfs {
		m.savedWorkflowIDs[wf.ID] = true
	}
	m.savedWorkflowIDsCollection = m.collection.Info.Name
	return m
}

// ensureSavedWorkflowIDs reloads the saved-workflow cache when the current collection changed.
// A collection without a file on disk (e.g. the merged view) simply has no saved workflows.
func (m Model) ensureSavedWorkflowIDs() Model {
	if m.collection == nil || m.savedWorkflowIDsCollection == m.collection.Info.Name {
		return m
	}
	_, wfs, err := m.discoverCollectionWorkflows()
	if err != nil && !errors.Is(err, errCollectionPathNotFound) {
		logger.LogError("ensureSavedWorkflowIDs", m.collection.Info.Name, err)
	}
	return m.cacheSavedWorkflowIDs(wfs)
}

// hasSavedFolderWorkflow reports whether the folder named name inside parentPath has a saved workflow.
func (m Model) hasSavedFolderWorkflow(parentPath []string, name string) bool {
	return m.savedWorkflowIDs[workflow.FolderWorkflowID(append(slices.Clone(parentPath), name))]
}
