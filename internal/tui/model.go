package tui

import (
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"

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
)

type EditType int

const (
	EditTypeNone EditType = iota
	EditTypeRequest
	EditTypeEnvVariable
	EditTypeCollectionVariable
	EditTypeFolderVariable
	EditTypeScript
)

type ScriptType int

const (
	ScriptTypePreRequest ScriptType = iota
	ScriptTypeTest
)

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

	return Model{
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
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
