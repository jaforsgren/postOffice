package tui

import (
	"context"
	"postOffice/internal/engine"
	"postOffice/internal/grpc"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/suggest"
	"postOffice/internal/workflow"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type ViewMode int

const (
	ModeCollections ViewMode = iota
	ModeRequests
	ModeResponse
	ModeInfo
	ModeEnvironments
	ModeVariables
	ModeEdit
	ModeChanges
	ModeJSON
	ModeLog
	ModeFileBrowser
	ModeGRPCReflect
	ModeWorkflows
	ModeWorkflowRun
	ModeWorkflowDetail
	ModeSavedResponses
	ModeHelp
	ModeRequestVariables
	ModeHistory
	ModeHistoryDiff
	ModeTiming
	ModeDependencyGraph
	ModeListeners
	ModeListenerMessage
)

type GRPCReflectPhase int

const (
	GRPCPhaseServices GRPCReflectPhase = iota
	GRPCPhaseMethods
)

const modeUnset ViewMode = -1

// workflowProgressBuffer is how many run state snapshots may queue before the
// runner blocks waiting for the TUI to render them.
const workflowProgressBuffer = 20

type EditType int

const (
	EditTypeNone EditType = iota
	EditTypeRequest
	EditTypeEnvVariable
	EditTypeCollectionVariable
	EditTypeFolderVariable
	EditTypeGRPCRequest
)

type RequestExecution struct {
	Status     string
	Timestamp  time.Time
	Duration   time.Duration
	Response   *http.Response
	TestResult *script.TestResult
	Inputs     requestInputs
}

type RequestCompleteMsg struct {
	ItemID          string
	Response        *http.Response
	TestResult      *script.TestResult
	Collection      *postman.Collection
	Environment     *postman.Environment
	EnvironmentName string
	Protocol        history.Protocol
	ItemName        string
	IsModified      bool
	Replayed        bool
	// Cancelled is set when the request was cancelled before it completed.
	Cancelled bool
}

type Model struct {
	parser *postman.Parser
	engine *engine.Engine
	// runEngine sends workflow and folder run steps. Its bash scripts get no
	// terminal: taking it over would hide the run's progress and swallow its
	// keys (ctrl+x, X) for every step, so their prompts read EOF instead.
	runEngine           *engine.Engine
	terminal            *shellTerminal
	commandRegistry     *CommandRegistry
	mode                ViewMode
	commandMode         bool
	commandInput        textinput.Model
	commandHistory      []string
	historyIndex        int
	commandSuggestion   string
	cursor              int
	items               []string
	currentItems        []postman.Item
	currentItemPaths    [][]string
	expandedFolders     map[string]bool
	collection          *postman.Collection
	breadcrumb          []string
	width               int
	height              int
	statusMessage       string
	statusIsError       bool
	lastResponse        *http.Response
	lastTestResult      *script.TestResult
	currentInfoItem     *postman.Item
	jsonContent         string
	scrollOffset        int
	searchMode          bool
	searchInput         textinput.Model
	searchActive        bool
	filteredItems       []string
	filteredIndices     []int
	allItems            []string
	allCurrentItems     []postman.Item
	allCurrentItemPaths [][]string
	// allVariables and allWorkflows hold the Variables and Workflows lists
	// while a search filters them.
	allVariables []postman.VariableSource
	allWorkflows []*workflow.Workflow
	environment  *postman.Environment
	// mergedCollectionActive/mergedEnvironmentActive mark m.collection/m.environment
	// as the synthetic, read-only result of Parser.MergedCollection/MergedEnvironment
	// rather than a real loaded one — used to block edits against it.
	mergedCollectionActive  bool
	mergedEnvironmentActive bool
	previousMode            ViewMode
	variables               []postman.VariableSource
	envVarCursor            int

	editType             EditType
	editRequest          *postman.Request
	editRequestSnapshot  *postman.Request
	editVariable         *postman.Variable
	editEnvVariable      *postman.EnvVariable
	editItemName         string
	editOriginalName     string
	editFieldCursor      int
	editFieldInput       textinput.Model
	editFieldTextArea    textarea.Model
	editFieldMode        bool
	modifiedItems        map[string]bool
	modifiedCollections  map[string]bool
	modifiedEnvironments map[string]bool
	modifiedRequests     map[string]*postman.Request
	editItemPath         []string
	editEnvironmentName  string

	pathParams        map[string]string
	requestPathParams map[string]map[string]string

	fieldCompletion fieldCompletion
	fieldValueIndex *suggest.Index

	editIsNewItem bool
	pendingYank   bool
	// responsePanelItem is the request the Response view was opened on with
	// i, whose definition it shows above the exchange; nil otherwise.
	responsePanelItem *postman.Item

	requestVarsItem        postman.Item
	requestVarsFolderPath  []string
	requestVarsCursor      int
	requestVarEdit         requestVarEditState
	requestVarChoiceCursor int
	requestVarsReturnMode  ViewMode
	// requestVarsScope makes the panel list the variables defined on the
	// collection or folder at requestVarsFolderPath, instead of the ones
	// requestVarsItem uses.
	requestVarsScope         bool
	requestTypeSelectionMode bool

	responseViewport viewport.Model
	infoViewport     viewport.Model
	jsonViewport     viewport.Model
	logsViewport     viewport.Model

	fileBrowserActive  bool
	fileBrowserCwd     string
	fileBrowserPath    []string
	fileBrowserCommand string

	requestExecutions  map[string]*RequestExecution
	lastExecutedItemID string
	// inFlightRequests cancels each request still being sent, by item ID.
	inFlightRequests map[string]context.CancelFunc

	splitViewActive bool
	topBarHidden    bool
	// splitFocus is the split view pane j/k scroll instead of the list.
	splitFocus      splitPane
	splitPaneScroll [splitPaneCount]int

	grpcEditEndpoint    string
	grpcEditMethod      string
	grpcEditTLS         bool
	grpcReflectServices []grpc.ServiceInfo
	grpcSelectedService int
	grpcReflectPhase    GRPCReflectPhase

	workflows []*workflow.Workflow
	// workflowOwners is the collection each listed workflow belongs to (see listWorkflows).
	workflowOwners map[*workflow.Workflow]string
	workflowCursor int
	// workflowStepCursor indexes workflowStepRows: steps and the sub-steps of expanded folder steps.
	workflowStepCursor int
	activeWorkflow     *workflow.Workflow
	// activeWorkflowCollection is the collection activeWorkflow belongs to (see activateWorkflow).
	activeWorkflowCollection string
	// expandedWorkflowSteps are the run IDs of expandedWorkflowID's expanded folder steps;
	// folderStepDefinitions caches the sub-workflow steps of those that have not run.
	expandedWorkflowID    string
	expandedWorkflowSteps map[string]bool
	folderStepDefinitions map[string][]workflow.Step
	workflowState         workflow.ExecutionState
	// partialRunBase is the previous run a partial run in progress merges into; nil otherwise.
	partialRunBase *workflow.ExecutionState
	workflowChan   <-chan WorkflowMsg
	// cancelWorkflowRun stops the workflow run in progress; nil when none is.
	cancelWorkflowRun context.CancelFunc
	// abortWorkflowStep aborts one running step of the run in progress (Runner.Abort); nil when none is.
	abortWorkflowStep func(runID string) bool
	workflowViewport  viewport.Model
	// workflowStepPanelOpen shows the selected step's logs, request and
	// response beside the workflow's steps; workflowStepPanelScroll scrolls it.
	workflowStepPanelOpen   bool
	workflowStepPanelScroll int
	// workflowPreviewOpen shows the selected workflow's steps and last run
	// beside the workflows list; workflowPreviewScroll scrolls it.
	workflowPreviewOpen   bool
	workflowPreviewScroll int
	addingWorkflowStep    bool

	// pendingConfirmation holds an action awaiting a y/n answer (folder runs, deletions);
	// while set, the next key press answers the prompt instead of reaching normal mode.
	pendingConfirmation *pendingConfirmation
	// runSettings are the :runopts options applied to every workflow/folder run.
	runSettings runSettings
	// savedWorkflowIDs maps each collection shown by savedWorkflowIDsCollection (one, or
	// every active one in the combined view) to its workflow IDs on disk, cached so the
	// requests list can mark folders that have a saved workflow.
	savedWorkflowIDs           map[string]map[string]bool
	savedWorkflowIDsCollection string
	// combinedWorkflowCollection is the collection whose workflows the workflow views use
	// in the combined view (see workflowCollectionName).
	combinedWorkflowCollection string

	savedResponses        []postman.SavedResponse
	savedResponseCursor   int
	savedResponseItemID   string
	savedResponseViewport viewport.Model
	viewingSavedResponse  bool

	helpViewport   viewport.Model
	helpShowAll    bool
	helpReturnMode ViewMode

	// undoHistory undoes and redoes the collection and environment edits saved from the TUI.
	undoHistory *undoHistory

	requestHistory       *history.Store
	requestHistoryCursor int
	requestHistoryScope  requestHistoryScope
	historyReturnMode    ViewMode
	historyDiffViewport  viewport.Model
	// listeners are the background watches of PEEK requests, by item ID.
	listeners map[string]*listener
	// listenedMessages are every listener's messages, oldest first.
	listenedMessages []listenedMessage
	listenerCursor   int
	// listenedMessagesAbove counts the messages that arrived above the
	// selection while it was held in place.
	listenedMessagesAbove   int
	listenerMessageViewport viewport.Model
	// listenersReturnMode is where the listeners view was opened from.
	listenersReturnMode ViewMode
	// listenerFilter is the item ID of the listener whose messages alone are
	// listed; empty lists every listener's.
	listenerFilter string
	// listenerQuery is the listeners view's search, in lower case.
	listenerQuery string
	// listenersPaused holds new messages in heldListenedMessages instead of
	// listing them.
	listenersPaused      bool
	heldListenedMessages []listenedMessage
	// unseenListenedMessages counts the messages that arrived while the
	// listeners view was closed.
	unseenListenedMessages int

	// lastRepeatableKey is the key of the last repeatable action, which "." repeats.
	lastRepeatableKey string

	// jumps is the jumplist of requests view locations, oldest first, with
	// jumpIndex the position ctrl+o/ctrl+] move from (len(jumps) when not
	// jumping). lastJumpLocation is where the requests view last was, and
	// jumping marks a move made by the jumplist itself, which isn't recorded.
	jumps            []jumpLocation
	jumpIndex        int
	lastJumpLocation jumpLocation
	jumping          bool

	// favorites maps a request's "collection/folder/.../name" identifier
	// (see requestItemID) to true when it's been favorited.
	favorites             map[string]bool
	viewingFavorites      bool
	favoritesReturnCursor int

	prerequisiteRun *prerequisiteRun
	dependencyGraph dependencyGraphView
}

// GRPCReflectMsg carries the result of an async gRPC server reflection call.
type GRPCReflectMsg struct {
	Services []grpc.ServiceInfo
	Err      error
}

// WorkflowMsg carries a workflow state snapshot from the async runner.
// Done is true when the workflow has finished.
type WorkflowMsg struct {
	State workflow.ExecutionState
	Done  bool
}

// workflowRun runs a workflow with runner until it finishes or ctx is cancelled.
type workflowRun func(ctx context.Context, runner *workflow.Runner, onChange func(workflow.ExecutionState)) workflow.ExecutionState

// waitForWorkflow returns a tea.Cmd that blocks until the run's next message arrives on ch.
func waitForWorkflow(ch <-chan WorkflowMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func NewModel(parser *postman.Parser, requestHistory *history.Store) Model {
	cmdInput := textinput.New()
	cmdInput.Placeholder = "Enter command..."
	cmdInput.CharLimit = 500

	searchInput := textinput.New()
	searchInput.Placeholder = "Search..."
	searchInput.CharLimit = 100

	editFieldInput := textinput.New()
	editFieldInput.CharLimit = 1000

	editFieldTextArea := textarea.New()
	editFieldTextArea.CharLimit = 50000

	terminal := newShellTerminal()
	httpExecutor := http.NewExecutor()

	m := Model{
		parser:                  parser,
		engine:                  engine.NewWithTerminal(httpExecutor, terminal),
		runEngine:               engine.New(httpExecutor),
		terminal:                terminal,
		commandRegistry:         NewCommandRegistry(),
		mode:                    ModeCollections,
		commandMode:             false,
		commandInput:            cmdInput,
		commandHistory:          []string{},
		historyIndex:            -1,
		commandSuggestion:       "",
		cursor:                  0,
		items:                   []string{},
		currentItems:            []postman.Item{},
		expandedFolders:         make(map[string]bool),
		breadcrumb:              []string{},
		previousMode:            modeUnset,
		searchInput:             searchInput,
		editFieldInput:          editFieldInput,
		editFieldTextArea:       editFieldTextArea,
		modifiedItems:           make(map[string]bool),
		modifiedCollections:     make(map[string]bool),
		modifiedEnvironments:    make(map[string]bool),
		modifiedRequests:        make(map[string]*postman.Request),
		pathParams:              make(map[string]string),
		requestPathParams:       make(map[string]map[string]string),
		responseViewport:        viewport.New(0, 0),
		infoViewport:            viewport.New(0, 0),
		jsonViewport:            viewport.New(0, 0),
		logsViewport:            viewport.New(0, 0),
		workflowViewport:        viewport.New(0, 0),
		savedResponseViewport:   viewport.New(0, 0),
		helpViewport:            viewport.New(0, 0),
		requestHistory:          requestHistory,
		undoHistory:             newUndoHistory(),
		historyReturnMode:       modeUnset,
		historyDiffViewport:     viewport.New(0, 0),
		listeners:               make(map[string]*listener),
		listenerMessageViewport: viewport.New(0, 0),
		listenersReturnMode:     modeUnset,
		requestExecutions:       make(map[string]*RequestExecution),
		inFlightRequests:        make(map[string]context.CancelFunc),
		favorites:               make(map[string]bool),
	}
	if ids, err := postman.LoadFavorites(); err == nil {
		for _, id := range ids {
			m.favorites[id] = true
		}
	}
	// Init waits for the listeners the session resumes.
	m, _ = m.restoreSession()
	return m
}

// Init waits for the listeners resumed from the previous session.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.waitForAllListeners(), m.terminal.waitForHandOff())
}
