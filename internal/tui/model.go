package tui

import (
	"postOffice/internal/grpc"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
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
)

type EditType int

const (
	EditTypeNone EditType = iota
	EditTypeRequest
	EditTypeEnvVariable
	EditTypeCollectionVariable
	EditTypeFolderVariable
	EditTypeScript
	EditTypeGRPCRequest
	EditTypeWorkflowStepScript
)

type ScriptType int

const (
	ScriptTypePreRequest ScriptType = iota
	ScriptTypeTest
	ScriptTypeStepPre
	ScriptTypeStepPost
)

type RequestExecution struct {
	Status     string
	Timestamp  time.Time
	Duration   time.Duration
	Response   *http.Response
	TestResult *script.TestResult
}

type RequestCompleteMsg struct {
	ItemID       string
	Response     *http.Response
	TestResult   *script.TestResult
	Collection   *postman.Collection
	Environment  *postman.Environment
	ItemName     string
	IsModified   bool
}

type Model struct {
	parser            *postman.Parser
	executor          *http.Executor
	commandRegistry   *CommandRegistry
	mode              ViewMode
	commandMode       bool
	commandInput      textinput.Model
	commandHistory    []string
	historyIndex      int
	commandSuggestion string
	cursor            int
	items             []string
	currentItems      []postman.Item
	collection        *postman.Collection
	breadcrumb        []string
	width             int
	height            int
	statusMessage     string
	lastResponse      *http.Response
	lastTestResult    *script.TestResult
	currentInfoItem   *postman.Item
	jsonContent       string
	scrollOffset      int
	searchMode        bool
	searchInput       textinput.Model
	searchActive      bool
	filteredItems     []string
	filteredIndices   []int
	allItems          []string
	allCurrentItems   []postman.Item
	environment       *postman.Environment
	previousMode      ViewMode
	variables         []postman.VariableSource
	envVarCursor      int

	editType             EditType
	editRequest          *postman.Request
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
	editCollectionName   string
	editEnvironmentName  string

	editScript          *postman.Script
	editScriptType      ScriptType
	editScriptItemName  string
	scriptSelectionMode bool

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

	grpcEditEndpoint    string
	grpcEditMethod      string
	grpcEditTLS         bool
	grpcReflectServices []grpc.ServiceInfo
	grpcSelectedService int
	grpcReflectPhase    int // 0 = services list, 1 = methods list

	workflows              []*workflow.Workflow
	workflowCursor         int
	workflowStepCursor     int
	activeWorkflow         *workflow.Workflow
	workflowState          workflow.ExecutionState
	workflowChan           <-chan workflow.ExecutionState
	workflowViewport       viewport.Model
	addingWorkflowStep     bool
	editingWorkflowStepIdx int

	savedResponses         []postman.SavedResponse
	savedResponseCursor    int
	savedResponseItemID    string
	savedResponseViewport  viewport.Model
	viewingSavedResponse   bool
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

// waitForWorkflow returns a tea.Cmd that blocks until the next state arrives on ch.
func waitForWorkflow(ch <-chan workflow.ExecutionState) tea.Cmd {
	return func() tea.Msg {
		state, ok := <-ch
		return WorkflowMsg{State: state, Done: !ok}
	}
}

func NewModel(parser *postman.Parser) Model {
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

	m := Model{
		parser:               parser,
		executor:             http.NewExecutor(),
		commandRegistry:      NewCommandRegistry(),
		mode:                 ModeCollections,
		commandMode:          false,
		commandInput:         cmdInput,
		commandHistory:       []string{},
		historyIndex:         -1,
		commandSuggestion:    "",
		cursor:               0,
		items:                []string{},
		currentItems:         []postman.Item{},
		breadcrumb:           []string{},
		statusMessage:        "Press : to enter command mode",
		searchInput:          searchInput,
		editFieldInput:       editFieldInput,
		editFieldTextArea:    editFieldTextArea,
		modifiedItems:        make(map[string]bool),
		modifiedCollections:  make(map[string]bool),
		modifiedEnvironments: make(map[string]bool),
		modifiedRequests:     make(map[string]*postman.Request),
		responseViewport:     viewport.New(0, 0),
		infoViewport:         viewport.New(0, 0),
		jsonViewport:         viewport.New(0, 0),
		logsViewport:         viewport.New(0, 0),
		workflowViewport:      viewport.New(0, 0),
		savedResponseViewport: viewport.New(0, 0),
		requestExecutions:     make(map[string]*RequestExecution),
	}
	m = m.restoreSession()
	m.statusMessage = "Press : to enter command mode"
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}
