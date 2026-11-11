package tui

import (
	"errors"
	"fmt"
	nethttp "net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/dependency"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const (
	runWithPrerequisitesKey = "P"
	editDependenciesKey     = "L"
)

var (
	errNoCollectionLoaded    = errors.New("no collection loaded")
	errCollectionFileUnknown = errors.New("collection file path unknown; dependencies are stored next to the collection file")
)

// dependencyScope is the collection whose sidecar declares the dependencies of
// requests under a view folder. viewPrefix is the part of view paths above that
// collection: its group folder in the combined view, nothing otherwise.
type dependencyScope struct {
	collection *postman.Collection
	viewPrefix []string
	filePath   string
}

func (m Model) dependencyScopeAt(viewFolderPath []string) (dependencyScope, error) {
	if m.collection == nil {
		return dependencyScope{}, errNoCollectionLoaded
	}
	collection, folderPath := m.collectionScope(viewFolderPath)
	collectionPath, exists := m.parser.GetCollectionPath(collection.Info.Name)
	if !exists {
		return dependencyScope{}, errCollectionFileUnknown
	}
	return dependencyScope{
		collection: collection,
		viewPrefix: slices.Clone(viewFolderPath[:len(viewFolderPath)-len(folderPath)]),
		filePath:   dependency.FilePath(collectionPath),
	}, nil
}

// requestPath is the graph key of a request at viewFolderPath.
func (s dependencyScope) requestPath(viewFolderPath []string, requestName string) string {
	return postman.JoinItemPath(append(slices.Clone(viewFolderPath[len(s.viewPrefix):]), requestName)...)
}

func (s dependencyScope) viewFolderPath(folderPath []string) []string {
	return append(slices.Clone(s.viewPrefix), folderPath...)
}

func (s dependencyScope) requestExists(requestPath string) bool {
	item, _, err := resolveItemPath(s.collection, requestPath)
	return err == nil && item.IsRequest()
}

func (s dependencyScope) loadGraph() (*dependency.Graph, error) {
	return dependency.Load(s.filePath)
}

func (s dependencyScope) loadValidGraph() (*dependency.Graph, error) {
	graph, err := s.loadGraph()
	if err != nil {
		return nil, err
	}
	if err := graph.Validate(s.requestExists); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", filepath.Base(s.filePath), err)
	}
	return graph, nil
}

// sessionDependencyState answers dependency staleness questions from the TUI session.
type sessionDependencyState struct {
	scope       dependencyScope
	executions  map[string]*RequestExecution
	identify    func(collectionName string, folderPath []string, requestName string) string
	environment *postman.Environment
	variables   []postman.VariableSource
}

func (m Model) dependencyState(scope dependencyScope) sessionDependencyState {
	return sessionDependencyState{
		scope:       scope,
		executions:  m.requestExecutions,
		identify:    m.getRequestIdentifierByPath,
		environment: m.environment,
		variables:   postman.GetAllVariables(scope.collection, nil, m.environment),
	}
}

func (s sessionDependencyState) execution(requestPath string) (*RequestExecution, bool) {
	names := postman.SplitItemPath(requestPath)
	if len(names) == 0 {
		return nil, false
	}
	last := len(names) - 1
	execution, exists := s.executions[s.identify(s.scope.collection.Info.Name, names[:last], names[last])]
	return execution, exists
}

func (s sessionDependencyState) LastSuccess(requestPath string) (time.Time, bool) {
	execution, exists := s.execution(requestPath)
	if !exists || executionFailure(execution.Response, execution.TestResult) != "" {
		return time.Time{}, false
	}
	return execution.Timestamp, true
}

// LastFailure leaves out a request still being sent, which has no response yet.
func (s sessionDependencyState) LastFailure(requestPath string) string {
	execution, exists := s.execution(requestPath)
	if !exists || execution.Response == nil {
		return ""
	}
	return executionFailure(execution.Response, execution.TestResult)
}

func (s sessionDependencyState) VariableSet(name string) bool {
	return variableValue(s.variables, name) != ""
}

func (s sessionDependencyState) ChangedInputs(requestPath string) []string {
	execution, exists := s.execution(requestPath)
	if !exists || execution.Inputs.values == nil {
		return nil
	}
	current := postman.GetAllVariables(s.scope.collection, execution.Inputs.folderPath, s.environment)
	var changed []string
	for _, name := range execution.Inputs.names {
		if variableValue(current, name) != execution.Inputs.values[name] {
			changed = append(changed, name)
		}
	}
	return changed
}

func (s sessionDependencyState) Now() time.Time {
	return time.Now()
}

// variableValue returns the resolved value of name, or "" when it is not defined.
func variableValue(variables []postman.VariableSource, name string) string {
	for _, v := range variables {
		if v.Key == name {
			return v.Value
		}
	}
	return ""
}

// requestInputs records the variables a request referenced, so a prerequisite
// becomes stale once one of them changes. names and folderPath (relative to the
// request's collection) are set when the request is sent, values when it completes.
type requestInputs struct {
	folderPath []string
	names      []string
	values     map[string]string
}

func sentRequestInputs(request *postman.Request, collectionFolderPath []string) requestInputs {
	return requestInputs{folderPath: slices.Clone(collectionFolderPath), names: postman.ReferencedVariables(request)}
}

// completedRequestInputs resolves the inputs recorded on the sending execution
// after the request ran, so values its own scripts set are not seen as later changes.
func (m Model) completedRequestInputs(sending *RequestExecution, collection *postman.Collection) requestInputs {
	if sending == nil || collection == nil || len(sending.Inputs.names) == 0 {
		return requestInputs{}
	}
	inputs := sending.Inputs
	variables := postman.GetAllVariables(collection, inputs.folderPath, m.environment)
	inputs.values = make(map[string]string, len(inputs.names))
	for _, name := range inputs.names {
		inputs.values[name] = variableValue(variables, name)
	}
	return inputs
}

// executionFailure describes why an execution does not count as successful, or returns "" when it does:
// sent (not skipped), no transport error, a 2xx status and no failed tests or script errors.
func executionFailure(response *internalhttp.Response, testResult *script.TestResult) string {
	switch {
	case response == nil:
		return "no response yet"
	case response.Skipped:
		return "skipped by pre-request script"
	case response.Error != nil:
		return response.Error.Error()
	case response.StatusCode < nethttp.StatusOK || response.StatusCode >= nethttp.StatusMultipleChoices:
		return fmt.Sprintf("status %d", response.StatusCode)
	case testResult != nil && testResult.HasFailures():
		return "tests failed"
	}
	return ""
}

// singleLineError flattens joined errors so they fit the one-line status bar.
func singleLineError(err error) string {
	return strings.ReplaceAll(err.Error(), "\n", "; ")
}

type prerequisiteStep struct {
	path           string
	itemID         string
	item           postman.Item
	viewFolderPath []string
	// isRetry marks the second attempt after a refresh_on status, which is never refreshed again.
	isRetry bool
}

// prerequisiteRun tracks a chain of stale prerequisites; the last step is the requested target.
type prerequisiteRun struct {
	graph   *dependency.Graph
	steps   []prerequisiteStep
	current int
}

func (r *prerequisiteRun) currentStep() prerequisiteStep { return r.steps[r.current] }
func (r *prerequisiteRun) target() prerequisiteStep      { return r.steps[len(r.steps)-1] }
func (r *prerequisiteRun) prerequisiteCount() int        { return len(r.steps) - 1 }
func (r *prerequisiteRun) atTarget() bool                { return r.current == len(r.steps)-1 }

func handleRunWithPrerequisitesKey(m Model) (Model, tea.Cmd) {
	item, ok := m.selectedItem()
	if !ok || !item.IsRequest() {
		m.statusMessage = "Select a request to run with its prerequisites"
		return m, nil
	}
	return m.runWithPrerequisites(item, m.folderPathForCursor())
}

func handleEditDependenciesKey(m Model) (Model, tea.Cmd) {
	return m.openVimForDependencies()
}

// runWithPrerequisites executes the stale prerequisites of item in dependency order,
// then item itself. Without stale prerequisites it is a plain execution.
func (m Model) runWithPrerequisites(item postman.Item, viewFolderPath []string) (Model, tea.Cmd) {
	if m.prerequisiteRun != nil {
		m.statusMessage = fmt.Sprintf("Prerequisites for %q are still running", m.prerequisiteRun.target().path)
		return m, nil
	}

	scope, err := m.dependencyScopeAt(viewFolderPath)
	if err != nil {
		m.statusMessage = "Cannot run prerequisites: " + err.Error()
		return m, nil
	}
	graph, err := scope.loadValidGraph()
	if err != nil {
		m.statusMessage = "Cannot run prerequisites: " + singleLineError(err)
		return m, nil
	}

	targetPath := scope.requestPath(viewFolderPath, item.Name)
	stale, err := graph.StalePrerequisites(targetPath, m.dependencyState(scope))
	if err != nil {
		m.statusMessage = "Cannot run prerequisites: " + err.Error()
		return m, nil
	}
	if len(stale) == 0 && !hasRefreshRules(graph.Node(targetPath)) {
		return m.executeRequest(item, viewFolderPath)
	}

	steps, err := m.prerequisiteSteps(scope, stale)
	if err != nil {
		m.statusMessage = "Cannot run prerequisites: " + err.Error()
		return m, nil
	}
	steps = append(steps, m.newPrerequisiteStep(targetPath, item, viewFolderPath))

	m.prerequisiteRun = &prerequisiteRun{graph: graph, steps: steps}
	return m.startPrerequisiteStep()
}

func hasRefreshRules(node dependency.Node) bool {
	return slices.ContainsFunc(node.Requires, func(required dependency.Requirement) bool { return len(required.RefreshOn) > 0 })
}

func (m Model) prerequisiteSteps(scope dependencyScope, paths []string) ([]prerequisiteStep, error) {
	steps := make([]prerequisiteStep, 0, len(paths)+1)
	for _, path := range paths {
		prerequisite, folderPath, err := resolveItemPath(scope.collection, path)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve prerequisite %q: %w", path, err)
		}
		steps = append(steps, m.newPrerequisiteStep(path, *prerequisite, scope.viewFolderPath(folderPath)))
	}
	return steps, nil
}

func (m Model) newPrerequisiteStep(path string, item postman.Item, viewFolderPath []string) prerequisiteStep {
	return prerequisiteStep{
		path:           path,
		itemID:         m.requestItemID(viewFolderPath, item.Name),
		item:           item,
		viewFolderPath: viewFolderPath,
	}
}

func (m Model) startPrerequisiteStep() (Model, tea.Cmd) {
	run := m.prerequisiteRun
	step := run.currentStep()

	var cmd tea.Cmd
	m, cmd = m.executeRequest(step.item, step.viewFolderPath)
	if cmd == nil {
		return m.stopPrerequisiteRun(fmt.Sprintf("Cannot execute %q (%s)", step.path, m.statusMessage)), nil
	}
	if !run.atTarget() {
		m.statusMessage = fmt.Sprintf("Running prerequisite %d/%d: %s (for %s)", run.current+1, run.prerequisiteCount(), step.path, run.target().path)
	}
	return m, cmd
}

// continuePrerequisiteRun advances the active chain when msg completes its current step.
func (m Model) continuePrerequisiteRun(msg RequestCompleteMsg) (Model, tea.Cmd) {
	run := m.prerequisiteRun
	if run == nil || msg.ItemID != run.currentStep().itemID {
		return m, nil
	}

	step := run.currentStep()
	if msg.Cancelled {
		return m.stopPrerequisiteRun(fmt.Sprintf("Cancelled %q", step.path)), nil
	}
	scope, err := m.dependencyScopeAt(run.target().viewFolderPath)
	if err != nil {
		return m.stopPrerequisiteRun(err.Error()), nil
	}
	if !step.isRetry && msg.Response != nil && msg.Response.Error == nil {
		refreshPaths, err := run.graph.RefreshPrerequisites(step.path, msg.Response.StatusCode, m.dependencyState(scope))
		if err != nil {
			return m.stopPrerequisiteRun(err.Error()), nil
		}
		if len(refreshPaths) > 0 {
			return m.refreshPrerequisiteRun(scope, refreshPaths, msg.Response.StatusCode)
		}
	}
	if run.atTarget() {
		m.prerequisiteRun = nil
		if count := run.prerequisiteCount(); count > 0 {
			m.statusMessage += fmt.Sprintf(" (after %d prerequisite(s))", count)
		}
		return m, nil
	}
	if reason := executionFailure(msg.Response, msg.TestResult); reason != "" {
		return m.stopPrerequisiteRun(fmt.Sprintf("Prerequisite %q failed (%s)", step.path, reason)), nil
	}
	if unset := run.graph.UnsetProducedVariables(step.path, m.dependencyState(scope)); len(unset) > 0 {
		return m.stopPrerequisiteRun(fmt.Sprintf("Prerequisite %q did not set %s", step.path, strings.Join(unset, ", "))), nil
	}

	next := *run
	next.current++
	m.prerequisiteRun = &next
	return m.startPrerequisiteStep()
}

// refreshPrerequisiteRun runs refreshPaths right after the current step, which
// answered with a refresh_on status, followed by one retry of that step.
func (m Model) refreshPrerequisiteRun(scope dependencyScope, refreshPaths []string, status int) (Model, tea.Cmd) {
	run := m.prerequisiteRun
	step := run.currentStep()
	refreshSteps, err := m.prerequisiteSteps(scope, refreshPaths)
	if err != nil {
		return m.stopPrerequisiteRun(err.Error()), nil
	}
	retry := step
	retry.isRetry = true

	next := *run
	next.steps = slices.Concat(run.steps[:run.current+1], refreshSteps, []prerequisiteStep{retry}, run.steps[run.current+1:])
	next.current++
	m.prerequisiteRun = &next

	m, cmd := m.startPrerequisiteStep()
	if cmd != nil {
		m.statusMessage = fmt.Sprintf("%q answered %d, refreshing %s; %s", step.path, status, strings.Join(refreshPaths, ", "), m.statusMessage)
	}
	return m, cmd
}

func (m Model) stopPrerequisiteRun(reason string) Model {
	m.statusMessage = fmt.Sprintf("%s; stopped before %q", reason, m.prerequisiteRun.target().path)
	m.prerequisiteRun = nil
	return m
}
