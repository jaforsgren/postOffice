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

var errCollectionPathNotFound = errors.New("collection path not found")

// folderRun is a folder to run as a workflow.
type folderRun struct {
	folder     postman.Item
	folderPath []string // full path including the folder's own name
}

// folderRunPrompt asks to run the folder, naming any non-default :runopts
// so they are confirmed too.
func folderRunPrompt(folderPath []string, settings runSettings) string {
	prompt := confirmationPrompt(fmt.Sprintf("Run folder '%s' as workflow?", workflow.FolderWorkflowName(folderPath)))
	if !settings.isDefault() {
		prompt += "  [" + settings.describe() + "]"
	}
	return prompt
}

// requestFolderRun asks for confirmation before running the folder at the cursor as a workflow.
func (m Model) requestFolderRun(folder postman.Item, parentPath []string) Model {
	if m.workflowChan != nil {
		m.statusMessage = "Workflow already running"
		return m
	}
	run := folderRun{
		folder:     folder,
		folderPath: append(slices.Clone(parentPath), folder.Name),
	}
	return m.askConfirmation(
		folderRunPrompt(run.folderPath, m.runSettings),
		fmt.Sprintf("Cancelled running folder '%s'", workflow.FolderWorkflowName(run.folderPath)),
		func(m Model) (Model, tea.Cmd) { return m.runFolderAsWorkflow(run) },
	)
}

// runFolderAsWorkflow runs the folder's saved workflow, generating and saving one first when the
// folder has none yet (or its requests changed and the saved one was never hand-edited).
func (m Model) runFolderAsWorkflow(run folderRun) (Model, tea.Cmd) {
	if m.collection == nil {
		m.statusMessage = "Load a collection first"
		return m, nil
	}
	collection, folderPath := m.collectionScope(run.folderPath)
	m, err := m.scopeWorkflowsTo(collection.Info.Name)
	if err != nil {
		return m.folderRunError(err), nil
	}

	generated, err := m.generateFolderWorkflow(collection, run.folder, folderPath)
	if err != nil {
		return m.folderRunError(err), nil
	}

	collectionPath, saved, err := m.discoverWorkflows(collection.Info.Name)
	if err != nil {
		return m.folderRunError(err), nil
	}

	wf, resolution := workflow.ResolveFolderWorkflow(workflow.FindWorkflow(saved, generated.ID), generated)
	if resolution.NeedsSave() {
		if err := workflow.SaveWorkflow(wf, workflow.WorkflowPath(collectionPath, wf.ID)); err != nil {
			return m.folderRunError(fmt.Errorf("failed to save workflow: %w", err)), nil
		}
	}
	if m, err = m.listWorkflows(); err != nil {
		return m.folderRunError(err), nil
	}

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

// generateFolderWorkflow generates the workflow running folder, at folderPath
// inside collection. A combined view group folder is a whole collection.
func (m Model) generateFolderWorkflow(collection *postman.Collection, folder postman.Item, folderPath []string) (*workflow.Workflow, error) {
	if len(folderPath) == 0 {
		return workflow.NewCollectionWorkflow(collection)
	}
	return workflow.NewFolderWorkflow(folder, folderPath)
}

// folderWorkflowSource gives folder steps the workflow running their folder on
// its own would, from the named collection's saved workflows; a collection
// without a file on disk has none saved, so its folders run generated ones.
func (m Model) folderWorkflowSource(collectionName string) workflow.FolderWorkflowSource {
	collection, _ := m.parser.GetCollection(collectionName)
	_, saved, err := m.discoverWorkflows(collectionName)
	if err != nil && !errors.Is(err, errCollectionPathNotFound) {
		logger.LogError("folderWorkflowSource", collectionName, err)
	}
	return workflow.SavedFolderWorkflows(collection, saved)
}

// discoverWorkflows loads every workflow saved next to the named collection's file.
func (m Model) discoverWorkflows(collectionName string) (string, []*workflow.Workflow, error) {
	collectionPath, exists := m.parser.GetCollectionPath(collectionName)
	if !exists {
		return "", nil, errCollectionPathNotFound
	}
	wfs, err := workflow.DiscoverCollectionWorkflows(collectionPath)
	return collectionPath, wfs, err
}

func (m Model) cacheSavedWorkflowIDs(collectionName string, wfs []*workflow.Workflow) Model {
	ids := make(map[string]bool, len(wfs))
	for _, wf := range wfs {
		ids[wf.ID] = true
	}
	if m.savedWorkflowIDs == nil {
		m.savedWorkflowIDs = make(map[string]map[string]bool)
	}
	m.savedWorkflowIDs[collectionName] = ids
	return m
}

// ensureSavedWorkflowIDs reloads the saved-workflow cache when the current collection changed.
// A collection without a file on disk simply has no saved workflows.
func (m Model) ensureSavedWorkflowIDs() Model {
	if m.collection == nil || m.savedWorkflowIDsCollection == m.collection.Info.Name {
		return m
	}
	m.savedWorkflowIDs = make(map[string]map[string]bool)
	for _, name := range m.shownCollectionNames() {
		_, wfs, err := m.discoverWorkflows(name)
		if err != nil && !errors.Is(err, errCollectionPathNotFound) {
			logger.LogError("ensureSavedWorkflowIDs", name, err)
		}
		m = m.cacheSavedWorkflowIDs(name, wfs)
	}
	m.savedWorkflowIDsCollection = m.collection.Info.Name
	return m
}

// shownCollectionNames are the collections the requests view shows.
func (m Model) shownCollectionNames() []string {
	if m.isViewingMergedCollection() {
		return m.parser.ActiveCollections()
	}
	return []string{m.collection.Info.Name}
}

// hasSavedFolderWorkflow reports whether the folder named name inside parentPath has a saved workflow.
func (m Model) hasSavedFolderWorkflow(parentPath []string, name string) bool {
	collection, folderPath := m.collectionScope(append(slices.Clone(parentPath), name))
	if len(folderPath) == 0 {
		return false
	}
	return m.savedWorkflowIDs[collection.Info.Name][workflow.FolderWorkflowID(folderPath)]
}
