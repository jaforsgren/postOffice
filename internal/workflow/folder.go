package workflow

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"postOffice/internal/postman"
)

// MaxFolderRunRequests stops a folder run whose pm.execution.setNextRequest
// calls loop forever (a retry that never becomes ready, a cycle of requests).
const MaxFolderRunRequests = 1000

const (
	slugSeparator         = "-"
	fallbackFolderSlug    = "folder"
	fallbackStepSlug      = "step"
	folderWorkflowVersion = 1
)

var (
	// ErrFolderHasNoRequests is returned when a folder (including its subfolders) holds no requests.
	ErrFolderHasNoRequests = errors.New("folder contains no requests")
	// ErrFolderNotFound is returned when a folder path does not exist in the collection.
	ErrFolderNotFound = errors.New("folder not found")
	// ErrRequestNotFound is returned when a request path does not name a request in the collection.
	ErrRequestNotFound = errors.New("request not found")
)

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
	return postman.JoinItemPath(folderPath...)
}

// FolderWorkflowID returns the stable workflow ID for a folder, e.g. "policies/TEST" → "policies-test".
func FolderWorkflowID(folderPath []string) string {
	return slugify(FolderWorkflowName(folderPath), fallbackFolderSlug)
}

// NewFolderWorkflow generates a workflow that runs every request under folder in collection order,
// depth-first through subfolders. folderPath is the folder's full path, including its own name.
func NewFolderWorkflow(folder postman.Item, folderPath []string) (*Workflow, error) {
	name := FolderWorkflowName(folderPath)
	return newRunWorkflow(folder.Items, folderPath, FolderWorkflowID(folderPath), name,
		fmt.Sprintf("Runs every request in folder %q in collection order", name))
}

// NewCollectionWorkflow generates a workflow that runs every request in the collection in order,
// like NewFolderWorkflow does for one folder.
func NewCollectionWorkflow(collection *postman.Collection) (*Workflow, error) {
	name := collection.Info.Name
	return newRunWorkflow(collection.Items, nil, slugify(name, fallbackFolderSlug), name,
		fmt.Sprintf("Runs every request in collection %q in collection order", name))
}

// NewFolderPathWorkflow generates the folder workflow for the folder at folderPath in collection.
func NewFolderPathWorkflow(collection *postman.Collection, folderPath []string) (*Workflow, error) {
	items := collection.Items
	for depth, name := range folderPath {
		folder := findItem(items, name)
		if folder == nil || folder.IsRequest() {
			return nil, fmt.Errorf("%w: %q", ErrFolderNotFound, FolderWorkflowName(folderPath[:depth+1]))
		}
		items = folder.Items
	}
	return NewFolderWorkflow(postman.Item{Name: FolderWorkflowName(folderPath), Items: items}, folderPath)
}

// NewRequestWorkflow generates a one-step workflow that runs the request at requestPath
// ("Folder/Sub/Request", with "\/" for a "/" inside a name) in collection.
func NewRequestWorkflow(collection *postman.Collection, requestPath string) (*Workflow, error) {
	parts := postman.SplitItemPath(requestPath)
	items := collection.Items
	var request *postman.Item
	for depth, name := range parts {
		request = findItem(items, name)
		if request == nil || (depth < len(parts)-1 && request.IsRequest()) {
			return nil, fmt.Errorf("%w: %q", ErrRequestNotFound, requestPath)
		}
		items = request.Items
	}
	if !request.IsRequest() {
		return nil, fmt.Errorf("%w: %q is a folder", ErrRequestNotFound, requestPath)
	}

	steps := []Step{{ID: slugify(request.Name, fallbackStepSlug), Request: requestPath}}
	return &Workflow{
		ID:          steps[0].ID,
		Name:        requestPath,
		Description: fmt.Sprintf("Runs request %q", requestPath),
		Version:     folderWorkflowVersion,
		Steps:       steps,
		Script:      PartialScript(steps, 0, len(steps)-1),
	}, nil
}

func findItem(items []postman.Item, name string) *postman.Item {
	for i := range items {
		if items[i].Name == name {
			return &items[i]
		}
	}
	return nil
}

func newRunWorkflow(items []postman.Item, folderPath []string, id, name, description string) (*Workflow, error) {
	refs := RequestRefs(items, folderPath)
	if len(refs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrFolderHasNoRequests, name)
	}

	steps := make([]Step, 0, len(refs))
	usedIDs := make(map[string]bool, len(refs))
	for _, ref := range refs {
		steps = append(steps, Step{
			ID:      uniqueStepID(ref.Item.Name, usedIDs),
			Request: ref.Path,
		})
	}

	return &Workflow{
		ID:          id,
		Name:        name,
		Description: description,
		Version:     folderWorkflowVersion,
		Steps:       steps,
		Script:      folderRunScript(steps),
	}, nil
}

// ResolveFolderWorkflow picks the workflow a folder run should use, given the saved workflow for the
// folder (nil when none exists) and one freshly generated from the folder's current requests.
func ResolveFolderWorkflow(saved, generated *Workflow) (*Workflow, FolderWorkflowResolution) {
	switch {
	case saved == nil:
		return generated, FolderWorkflowCreated
	case sameStepRequests(saved.Steps, generated.Steps) && !isLegacyFolderWorkflow(saved):
		return saved, FolderWorkflowReused
	case isUnmodifiedFolderWorkflow(saved):
		return generated, FolderWorkflowRegenerated
	default:
		return saved, FolderWorkflowCustomizedStale
	}
}

// RequestRef is a request together with the "Folder/Sub/Request" path that runs and workflow
// steps address it by. Names containing "/" are escaped in Path (see postman.JoinItemPath).
type RequestRef struct {
	Path string
	Item *postman.Item
}

// RequestRefs lists every request under items depth-first in collection order. parentPath is the
// folder path items live in, so returned paths are complete.
func RequestRefs(items []postman.Item, parentPath []string) []RequestRef {
	var refs []RequestRef
	for i := range items {
		item := &items[i]
		itemPath := append(append([]string{}, parentPath...), item.Name)
		if item.IsRequest() {
			refs = append(refs, RequestRef{Path: postman.JoinItemPath(itemPath...), Item: item})
			continue
		}
		refs = append(refs, RequestRefs(item.Items, itemPath)...)
	}
	return refs
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

// folderRunScript runs steps in order like Postman's Collection Runner: a step whose request calls
// pm.execution.setNextRequest jumps to that step, null ends the run, and a target outside the run
// fails it.
func folderRunScript(steps []Step) string {
	ids := make([]string, len(steps))
	for i, step := range steps {
		ids[i] = strconv.Quote(step.ID)
	}
	return fmt.Sprintf(`export default async function(wf, pm) {
  const steps = [%s];
  let next = 0;
  for (let runs = 0; next < steps.length; runs++) {
    if (runs >= %d) {
      wf.fail("stopped after %d requests: likely a pm.execution.setNextRequest loop");
    }
    const result = await wf.run(steps[next]);
    if (result.nextRequest === undefined) {
      next++;
    } else if (result.nextRequest === null) {
      break;
    } else if (result.nextStep === undefined) {
      wf.fail("setNextRequest target " + JSON.stringify(result.nextRequest) + " is not in this run");
    } else {
      next = steps.indexOf(result.nextStep);
    }
  }
}`, strings.Join(ids, ", "), MaxFolderRunRequests, MaxFolderRunRequests)
}

// legacyFolderRunScript is the plain sequential script folder workflows were generated with before
// they followed setNextRequest. Saved workflows still using it are regenerated, not kept.
func legacyFolderRunScript(steps []Step) string {
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

// isUnmodifiedFolderWorkflow reports whether wf is still exactly what NewFolderWorkflow produces, or
// produced before folder runs followed setNextRequest, for its own steps: a generated script and no
// per-step scripts.
func isUnmodifiedFolderWorkflow(wf *Workflow) bool {
	return hasNoStepScripts(wf) && (hasScript(wf, folderRunScript(wf.Steps)) || hasScript(wf, legacyFolderRunScript(wf.Steps)))
}

func isLegacyFolderWorkflow(wf *Workflow) bool {
	return hasNoStepScripts(wf) && hasScript(wf, legacyFolderRunScript(wf.Steps))
}

func hasNoStepScripts(wf *Workflow) bool {
	for _, step := range wf.Steps {
		if step.PreScript != "" || step.PostScript != "" {
			return false
		}
	}
	return true
}

func hasScript(wf *Workflow, script string) bool {
	return strings.TrimSpace(wf.Script) == strings.TrimSpace(script)
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
