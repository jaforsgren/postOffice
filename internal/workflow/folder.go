package workflow

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"postOffice/internal/postman"
)

// RequestPathSeparator separates folder and request names in a step's request path.
const RequestPathSeparator = "/"

const (
	slugSeparator         = "-"
	fallbackFolderSlug    = "folder"
	fallbackStepSlug      = "step"
	folderWorkflowVersion = 1
)

// ErrFolderHasNoRequests is returned when a folder (including its subfolders) holds no requests.
var ErrFolderHasNoRequests = errors.New("folder contains no requests")

// UnaddressableNameError reports a folder or request whose name contains RequestPathSeparator.
// Step request paths are resolved by splitting on that separator, so such an item cannot be a step.
type UnaddressableNameError struct {
	Name string
}

func (e *UnaddressableNameError) Error() string {
	return fmt.Sprintf("%q contains %q, so a workflow step cannot address it", e.Name, RequestPathSeparator)
}

// FolderWorkflowResolution describes which workflow a folder run uses and whether it must be saved.
type FolderWorkflowResolution int

const (
	// FolderWorkflowCreated means no workflow existed for the folder; the generated one is used.
	FolderWorkflowCreated FolderWorkflowResolution = iota
	// FolderWorkflowReused means the saved workflow still matches the folder's requests.
	FolderWorkflowReused
	// FolderWorkflowRegenerated means the folder's requests changed and the saved workflow was
	// never hand-edited, so it is replaced by a freshly generated one.
	FolderWorkflowRegenerated
	// FolderWorkflowCustomizedStale means the folder's requests changed but the saved workflow was
	// hand-edited, so it is kept as-is rather than overwriting the user's changes.
	FolderWorkflowCustomizedStale
)

// NeedsSave reports whether the resolved workflow must be written to disk.
func (r FolderWorkflowResolution) NeedsSave() bool {
	return r == FolderWorkflowCreated || r == FolderWorkflowRegenerated
}

// FolderWorkflowName returns the workflow name for a folder: its full path, e.g. "policies/TEST".
func FolderWorkflowName(folderPath []string) string {
	return strings.Join(folderPath, RequestPathSeparator)
}

// FolderWorkflowID returns the stable workflow ID for a folder, e.g. "policies/TEST" → "policies-test".
func FolderWorkflowID(folderPath []string) string {
	return slugify(FolderWorkflowName(folderPath), fallbackFolderSlug)
}

// NewFolderWorkflow generates a workflow that runs every request under folder in collection order,
// depth-first through subfolders. folderPath is the folder's full path, including its own name.
func NewFolderWorkflow(folder postman.Item, folderPath []string) (*Workflow, error) {
	for _, name := range folderPath {
		if strings.Contains(name, RequestPathSeparator) {
			return nil, &UnaddressableNameError{Name: name}
		}
	}

	var requestPaths [][]string
	if err := collectRequestPaths(folder.Items, folderPath, &requestPaths); err != nil {
		return nil, err
	}
	if len(requestPaths) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrFolderHasNoRequests, FolderWorkflowName(folderPath))
	}

	steps := make([]Step, 0, len(requestPaths))
	usedIDs := make(map[string]bool, len(requestPaths))
	for _, path := range requestPaths {
		steps = append(steps, Step{
			ID:      uniqueStepID(path[len(path)-1], usedIDs),
			Request: strings.Join(path, RequestPathSeparator),
		})
	}

	name := FolderWorkflowName(folderPath)
	return &Workflow{
		ID:          FolderWorkflowID(folderPath),
		Name:        name,
		Description: fmt.Sprintf("Runs every request in folder %q in collection order", name),
		Version:     folderWorkflowVersion,
		Steps:       steps,
		Script:      sequentialScript(steps),
	}, nil
}

// ResolveFolderWorkflow picks the workflow a folder run should use, given the saved workflow for the
// folder (nil when none exists) and one freshly generated from the folder's current requests.
func ResolveFolderWorkflow(saved, generated *Workflow) (*Workflow, FolderWorkflowResolution) {
	switch {
	case saved == nil:
		return generated, FolderWorkflowCreated
	case sameStepRequests(saved.Steps, generated.Steps):
		return saved, FolderWorkflowReused
	case isUnmodifiedFolderWorkflow(saved):
		return generated, FolderWorkflowRegenerated
	default:
		return saved, FolderWorkflowCustomizedStale
	}
}

func collectRequestPaths(items []postman.Item, parentPath []string, out *[][]string) error {
	for _, item := range items {
		if strings.Contains(item.Name, RequestPathSeparator) {
			return &UnaddressableNameError{Name: item.Name}
		}
		itemPath := append(append([]string{}, parentPath...), item.Name)
		if item.IsRequest() {
			*out = append(*out, itemPath)
			continue
		}
		if err := collectRequestPaths(item.Items, itemPath, out); err != nil {
			return err
		}
	}
	return nil
}

func uniqueStepID(requestName string, usedIDs map[string]bool) string {
	base := slugify(requestName, fallbackStepSlug)
	id := base
	for suffix := 2; usedIDs[id]; suffix++ {
		id = fmt.Sprintf("%s%s%d", base, slugSeparator, suffix)
	}
	usedIDs[id] = true
	return id
}

func sequentialScript(steps []Step) string {
	return PartialScript(steps, 0, len(steps)-1)
}

func sameStepRequests(a, b []Step) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Request != b[i].Request {
			return false
		}
	}
	return true
}

// isUnmodifiedFolderWorkflow reports whether wf is still exactly what NewFolderWorkflow produces for
// its own steps: a plain sequential script and no per-step scripts.
func isUnmodifiedFolderWorkflow(wf *Workflow) bool {
	for _, step := range wf.Steps {
		if step.PreScript != "" || step.PostScript != "" {
			return false
		}
	}
	return strings.TrimSpace(wf.Script) == strings.TrimSpace(sequentialScript(wf.Steps))
}

// slugify lowercases s and joins its runs of letters and digits with slugSeparator.
// It returns fallback when s has no letters or digits.
func slugify(s, fallback string) string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) == 0 {
		return fallback
	}
	return strings.Join(words, slugSeparator)
}
