package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadWorkflow loads and parses a single workflow YAML file.
func LoadWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow file: %w", err)
	}

	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse workflow YAML: %w", err)
	}

	if wf.ID == "" {
		return nil, fmt.Errorf("workflow file is missing required field 'id'")
	}

	return &wf, nil
}

// WorkflowFile is a workflow together with the file it was loaded from.
type WorkflowFile struct {
	Path     string
	Workflow *Workflow
}

// DiscoverWorkflowFiles scans dir (not its subfolders) for *.yaml and *.yml files and loads each
// as a Workflow, keeping its file path. Returns a nil slice (no error) if dir does not exist.
func DiscoverWorkflowFiles(dir string) ([]WorkflowFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workflows directory: %w", err)
	}

	var files []WorkflowFile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, workflowFileExtension) && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		wf, err := LoadWorkflow(path)
		if err != nil {
			logger.LogError("DiscoverWorkflows", path, err)
			continue
		}
		files = append(files, WorkflowFile{Path: path, Workflow: wf})
	}

	return files, nil
}

// workflowFileExtension is the extension workflows are saved with; .yml files are read too.
const workflowFileExtension = ".yaml"

// collectionFileSuffixes are dropped from a collection's file name to name its workflows folder.
var collectionFileSuffixes = []string{".postman_collection.json", ".json"}

// WorkflowsDir returns the workflows directory beside a collection file, shared by every collection
// in that directory: it holds one folder per collection (see CollectionWorkflowsDir).
// E.g. "/path/to/MyCollection.json" → "/path/to/workflows".
func WorkflowsDir(collectionPath string) string {
	return filepath.Join(filepath.Dir(collectionPath), "workflows")
}

// CollectionWorkflowsDir returns the collection's own workflows folder, so collections in one
// directory never see or overwrite each other's workflows.
// E.g. "/path/to/SAPI.postman_collection.json" → "/path/to/workflows/SAPI".
func CollectionWorkflowsDir(collectionPath string) string {
	name := filepath.Base(collectionPath)
	for _, suffix := range collectionFileSuffixes {
		if trimmed, found := strings.CutSuffix(name, suffix); found {
			name = trimmed
			break
		}
	}
	return filepath.Join(WorkflowsDir(collectionPath), name)
}

// WorkflowPath returns the file the collection's workflow id is saved in: its own folder's, unless
// only a legacy shared file has that id, so editing a workflow saves it where it lives.
func WorkflowPath(collectionPath, id string) string {
	own := filepath.Join(CollectionWorkflowsDir(collectionPath), id+workflowFileExtension)
	legacy := filepath.Join(WorkflowsDir(collectionPath), id+workflowFileExtension)
	if !fileExists(own) && fileExists(legacy) {
		return legacy
	}
	return own
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DiscoverCollectionWorkflowFiles loads the collection's workflows: those in its own folder, then
// the legacy ones directly in the shared workflows directory, which every collection beside it
// sees, unless its own folder has one with the same id.
func DiscoverCollectionWorkflowFiles(collectionPath string) ([]WorkflowFile, error) {
	own, err := DiscoverWorkflowFiles(CollectionWorkflowsDir(collectionPath))
	if err != nil {
		return nil, err
	}
	shared, err := DiscoverWorkflowFiles(WorkflowsDir(collectionPath))
	if err != nil {
		return nil, err
	}
	for _, file := range shared {
		if !slices.ContainsFunc(own, func(o WorkflowFile) bool { return o.Workflow.ID == file.Workflow.ID }) {
			own = append(own, file)
		}
	}
	return own, nil
}

// DiscoverCollectionWorkflows is DiscoverCollectionWorkflowFiles without the file paths.
func DiscoverCollectionWorkflows(collectionPath string) ([]*Workflow, error) {
	files, err := DiscoverCollectionWorkflowFiles(collectionPath)
	var workflows []*Workflow
	for _, file := range files {
		workflows = append(workflows, file.Workflow)
	}
	return workflows, err
}

// NewWorkflow returns a skeleton workflow template for the given ID.
func NewWorkflow(id string) *Workflow {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	name := strings.Join(parts, " ")

	return &Workflow{
		ID:     id,
		Name:   name,
		Steps:  []Step{},
		Script: "export default async function (wf, pm) {\n  // start here\n}\n",
	}
}

// PartialScript builds a script that runs a contiguous slice of steps by ID.
// The range is [fromIdx, toIdx] inclusive. Steps outside the range are skipped silently.
// The result is a valid workflow script that can be assigned to Workflow.Script.
func PartialScript(steps []Step, fromIdx, toIdx int) string {
	var sb strings.Builder
	sb.WriteString("export default async function(wf, pm) {\n")
	for i := fromIdx; i <= toIdx && i < len(steps); i++ {
		sb.WriteString(fmt.Sprintf("  await wf.run(%q);\n", steps[i].ID))
	}
	sb.WriteString("}")
	return sb.String()
}

// SaveWorkflow writes a workflow to path as YAML, creating parent directories if needed.
func SaveWorkflow(wf *Workflow, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
