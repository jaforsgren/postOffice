package workflow

import (
	"errors"

	"postOffice/internal/postman"
)

// ErrNoCollection is returned when a run needs the collection but has none.
var ErrNoCollection = errors.New("no collection loaded")

// FolderWorkflowSource returns the workflow a folder step runs for the folder at folderPath.
type FolderWorkflowSource func(folderPath []string) (*Workflow, error)

// GeneratedFolderWorkflows generates every folder's workflow from collection, ignoring saved ones.
func GeneratedFolderWorkflows(collection *postman.Collection) FolderWorkflowSource {
	return func(folderPath []string) (*Workflow, error) {
		if collection == nil {
			return nil, ErrNoCollection
		}
		return NewFolderPathWorkflow(collection, folderPath)
	}
}

// SavedFolderWorkflows runs the workflow running each folder on its own would: its saved workflow
// when that still matches the folder or was hand-edited, else a generated one (see
// ResolveFolderWorkflow). Nothing is saved.
func SavedFolderWorkflows(collection *postman.Collection, saved []*Workflow) FolderWorkflowSource {
	generate := GeneratedFolderWorkflows(collection)
	return func(folderPath []string) (*Workflow, error) {
		generated, err := generate(folderPath)
		if err != nil {
			return nil, err
		}
		wf, _ := ResolveFolderWorkflow(FindWorkflow(saved, generated.ID), generated)
		return wf, nil
	}
}

// FindWorkflow returns the workflow with id in wfs, or nil.
func FindWorkflow(wfs []*Workflow, id string) *Workflow {
	for _, wf := range wfs {
		if wf.ID == id {
			return wf
		}
	}
	return nil
}
