package tui

import (
	"errors"
	"slices"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

var errCombinedWorkflowScope = errors.New("move the cursor onto a collection first: in the combined view workflows belong to one collection")

// workflowCollectionName is the collection whose workflow files the workflow
// views read and write. The combined tree has no file of its own, so there it
// is the selected workflow's collection in the workflows list, else the
// collection scopeWorkflowsTo picked.
func (m Model) workflowCollectionName() string {
	if m.isViewingMergedCollection() {
		if wf, ok := m.selectedListWorkflow(); ok && m.mode == ModeWorkflows {
			return m.workflowOwners[wf]
		}
		return m.combinedWorkflowCollection
	}
	if m.collection == nil {
		return ""
	}
	return m.collection.Info.Name
}

// workflowCollection is the collection workflow steps resolve against; nil
// when none is loaded or, in the combined view, none was picked.
func (m Model) workflowCollection() *postman.Collection {
	if !m.isViewingMergedCollection() {
		return m.collection
	}
	collection, _ := m.parser.GetCollection(m.workflowCollectionName())
	return collection
}

// listWorkflows fills the workflows list from disk: the workflow collection's
// workflows, or in the combined view every active collection's, each owned by
// its collection. Collections without a file have none.
func (m Model) listWorkflows() (Model, error) {
	names := []string{m.workflowCollectionName()}
	if m.isViewingMergedCollection() {
		names = m.parser.ActiveCollections()
	}
	var listed []*workflow.Workflow
	owners := make(map[*workflow.Workflow]string)
	for _, name := range names {
		_, wfs, err := m.discoverWorkflows(name)
		if errors.Is(err, errCollectionPathNotFound) && m.isViewingMergedCollection() {
			continue
		}
		if err != nil {
			return m, err
		}
		for _, wf := range wfs {
			owners[wf] = name
		}
		listed = append(listed, wfs...)
		m = m.cacheSavedWorkflowIDs(name, wfs)
	}
	m.workflows = listed
	m.workflowOwners = owners
	m.workflowCursor = min(m.workflowCursor, max(len(listed)-1, 0))
	return m, nil
}

// findListedWorkflow is the listed workflow of collectionName with the given ID or name.
func (m Model) findListedWorkflow(collectionName, idOrName string) (*workflow.Workflow, bool) {
	for _, wf := range m.workflows {
		if m.workflowOwners[wf] == collectionName && (wf.ID == idOrName || wf.Name == idOrName) {
			return wf, true
		}
	}
	return nil, false
}

// scopeWorkflowsToCursor picks, in the combined view, the collection holding
// the cursor's item as the one whose workflows the workflow commands use.
// From the workflow views it keeps the collection already picked.
func (m Model) scopeWorkflowsToCursor() (Model, error) {
	if !m.isViewingMergedCollection() {
		return m, nil
	}
	if m.isWorkflowMode() && m.combinedWorkflowCollection != "" {
		return m, nil
	}
	return m.scopeWorkflowsTo(m.cursorCollectionName())
}

func (m Model) scopeWorkflowsTo(collectionName string) (Model, error) {
	if !m.isViewingMergedCollection() {
		return m, nil
	}
	if !m.parser.IsCollectionActive(collectionName) {
		return m, errCombinedWorkflowScope
	}
	m.combinedWorkflowCollection = collectionName
	return m, nil
}

func (m Model) isWorkflowMode() bool {
	return m.mode == ModeWorkflows || m.mode == ModeWorkflowDetail || m.mode == ModeWorkflowRun
}

// cursorCollectionName is the combined view's group folder holding the
// cursor's item, or the group folder under the cursor at the top level.
func (m Model) cursorCollectionName() string {
	if path := m.requestsViewFolderPath(); len(path) > 0 {
		return path[0]
	}
	if item, ok := m.selectedItem(); ok {
		return item.Name
	}
	return ""
}

// workflowStepItemID is the run state key of a workflow step's request, the
// same one running that request from the requests view uses.
func (m Model) workflowStepItemID(requestPath string) string {
	names := postman.SplitItemPath(requestPath)
	if len(names) == 0 {
		return ""
	}
	return m.getRequestIdentifierByPath(m.workflowCollectionName(), names[:len(names)-1], names[len(names)-1])
}

// workflowViewPath is where folderPath inside the workflow collection sits in
// the current view: under its group folder in the combined view.
func (m Model) workflowViewPath(folderPath []string) []string {
	if !m.isViewingMergedCollection() {
		return slices.Clone(folderPath)
	}
	return append([]string{m.combinedWorkflowCollection}, folderPath...)
}
