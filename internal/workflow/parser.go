package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
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

// DiscoverWorkflows scans dir for *.yaml and *.yml files and loads each as a Workflow.
// Returns nil slice (no error) if the directory does not exist.
func DiscoverWorkflows(dir string) ([]*Workflow, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read workflows directory: %w", err)
	}

	var workflows []*Workflow
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		wf, err := LoadWorkflow(filepath.Join(dir, name))
		if err != nil {
			logger.LogError("DiscoverWorkflows", filepath.Join(dir, name), err)
			continue
		}
		workflows = append(workflows, wf)
	}

	return workflows, nil
}

// WorkflowsDir returns the workflows directory for a given collection file path.
// E.g. "/path/to/MyCollection.json" → "/path/to/workflows".
func WorkflowsDir(collectionPath string) string {
	return filepath.Join(filepath.Dir(collectionPath), "workflows")
}

// WorkflowPath returns the file path for a workflow given the collection path and workflow ID.
func WorkflowPath(collectionPath, id string) string {
	return filepath.Join(WorkflowsDir(collectionPath), id+".yaml")
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
		ID:    id,
		Name:  name,
		Steps: []Step{},
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
