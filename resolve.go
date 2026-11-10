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

// resolvedWorkflow is a workflow file and, when it was found by name, the
// collection it was saved next to.
type resolvedWorkflow struct {
	path           string
	collectionPath string
}

// resolveWorkflowRef turns a workflow file path, or the ID, name or file name
// of a workflow saved next to collectionPath (or, without one, next to any
// loaded collection), into a file path.
func resolveWorkflowRef(ref, collectionPath string) (resolvedWorkflow, error) {
	if path, ok := existingFile(ref); ok {
		return resolvedWorkflow{path: path}, nil
	}

	collectionPaths := []string{collectionPath}
	if collectionPath == "" {
		parser, err := loadedState()
		if err != nil {
			return resolvedWorkflow{}, err
		}
		collectionPaths = nil
		for _, name := range parser.ListCollections() {
			path, _ := parser.GetCollectionPath(name)
			collectionPaths = append(collectionPaths, path)
		}
	}

	var candidates []namedFile
	owners := make(map[string]string)
	for _, owner := range collectionPaths {
		files, err := workflow.DiscoverWorkflowFiles(workflow.WorkflowsDir(owner))
		if err != nil {
			return resolvedWorkflow{}, err
		}
		for _, file := range files {
			candidates = append(candidates, namedFile{names: []string{file.Workflow.ID, file.Workflow.Name, fileStem(file.Path, "")}, path: file.Path})
			owners[file.Path] = owner
		}
	}

	path, err := resolveNamedFile(ref, workflowReference, candidates)
	if err != nil {
		return resolvedWorkflow{}, err
	}
	return resolvedWorkflow{path: path, collectionPath: owners[path]}, nil
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
