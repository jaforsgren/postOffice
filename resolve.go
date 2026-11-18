package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

// Postman's export suffixes, dropped from a file name to get the name it can
// be referred to by, e.g. "SAPI.postman_collection.json" → "SAPI".
const (
	collectionExportSuffix  = ".postman_collection.json"
	environmentExportSuffix = ".postman_environment.json"
)

var (
	errUnknownReference   = errors.New("no such file, and nothing loaded by that name")
	errAmbiguousReference = errors.New("more than one loaded file has that name")
)

// namedFile is a loaded file that can be referred to by name.
type namedFile struct {
	names []string
	path  string
}

// referenceKind names what a reference points at, for error messages.
type referenceKind string

const (
	collectionReference  referenceKind = "collection"
	environmentReference referenceKind = "environment"
	workflowReference    referenceKind = "workflow"
)

// resolveCollectionRef turns a collection file path, or the name of a loaded
// collection (its own name or file name without extension), into a file path.
func resolveCollectionRef(ref string) (string, error) {
	if path, ok := existingFile(ref); ok {
		return path, nil
	}
	parser, err := loadedState()
	if err != nil {
		return "", err
	}
	return resolveNamedFile(ref, collectionReference, loadedFiles(parser.ListCollections(), parser.GetCollectionPath, collectionExportSuffix))
}

// resolveEnvironmentRef is resolveCollectionRef for environments.
func resolveEnvironmentRef(ref string) (string, error) {
	if path, ok := existingFile(ref); ok {
		return path, nil
	}
	parser, err := loadedState()
	if err != nil {
		return "", err
	}
	return resolveNamedFile(ref, environmentReference, loadedFiles(parser.ListEnvironments(), parser.GetEnvironmentPath, environmentExportSuffix))
}

// resolveWorkflowRef turns a workflow file path, or the ID, name or file name
// of a workflow saved next to collectionPath (or, without one, next to any
// loaded collection), into a file path.
func resolveWorkflowRef(ref, collectionPath string) (string, error) {
	if path, ok := existingFile(ref); ok {
		return path, nil
	}

	collectionPaths := []string{collectionPath}
	if collectionPath == "" {
		parser, err := loadedState()
		if err != nil {
			return "", err
		}
		collectionPaths = loadedCollectionPaths(parser)
	}

	saved, err := workflowsSavedNextTo(collectionPaths)
	if err != nil {
		return "", err
	}
	candidates := make([]namedFile, 0, len(saved))
	for _, file := range saved {
		candidates = append(candidates, namedFile{names: []string{file.workflow.Workflow.ID, file.workflow.Workflow.Name, fileStem(file.workflow.Path, "")}, path: file.workflow.Path})
	}
	return resolveNamedFile(ref, workflowReference, candidates)
}

// savedWorkflow is a workflow file and the collections it is saved next to.
type savedWorkflow struct {
	workflow        workflow.WorkflowFile
	collectionPaths []string
}

// workflowsSavedNextTo lists the workflows of collectionPaths, once per file:
// legacy files directly in a shared workflows directory belong to every
// collection beside it.
func workflowsSavedNextTo(collectionPaths []string) ([]savedWorkflow, error) {
	var saved []savedWorkflow
	indexByPath := make(map[string]int)
	for _, collectionPath := range collectionPaths {
		files, err := workflow.DiscoverCollectionWorkflowFiles(collectionPath)
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			if i, seen := indexByPath[file.Path]; seen {
				saved[i].collectionPaths = append(saved[i].collectionPaths, collectionPath)
				continue
			}
			indexByPath[file.Path] = len(saved)
			saved = append(saved, savedWorkflow{workflow: file, collectionPaths: []string{collectionPath}})
		}
	}
	return saved, nil
}

func loadedCollectionPaths(parser *postman.Parser) []string {
	var paths []string
	for _, name := range parser.ListCollections() {
		path, _ := parser.GetCollectionPath(name)
		paths = append(paths, path)
	}
	return paths
}

func loadedFiles(names []string, pathOf func(string) (string, bool), exportSuffix string) []namedFile {
	files := make([]namedFile, 0, len(names))
	for _, name := range names {
		path, _ := pathOf(name)
		files = append(files, namedFile{names: []string{name, fileStem(path, exportSuffix)}, path: path})
	}
	return files
}

// resolveNamedFile picks the one candidate named ref, ignoring case.
func resolveNamedFile(ref string, kind referenceKind, candidates []namedFile) (string, error) {
	var matches []string
	for _, candidate := range candidates {
		for _, name := range candidate.names {
			if strings.EqualFold(name, ref) {
				matches = append(matches, candidate.path)
				break
			}
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", usageErrorFor(errUnknownReference, "%s %q", kind, ref)
	default:
		return "", usageErrorFor(errAmbiguousReference, "%s %q: %s; pass the file path", kind, ref, strings.Join(matches, ", "))
	}
}

func existingFile(ref string) (string, bool) {
	path, err := postman.ExpandPath(ref)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir()
}

// fileStem is path's file name without exportSuffix, or without its
// extension when it doesn't end in exportSuffix.
func fileStem(path, exportSuffix string) string {
	base := filepath.Base(path)
	if exportSuffix != "" && strings.HasSuffix(base, exportSuffix) {
		return strings.TrimSuffix(base, exportSuffix)
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}
