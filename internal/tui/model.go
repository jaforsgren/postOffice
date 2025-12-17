package tui

import (
	"postOffice/internal/http"
	"postOffice/internal/postman"

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
)

type EditType int

const (
	EditTypeNone EditType = iota
	EditTypeRequest
	EditTypeEnvVariable
	EditTypeCollectionVariable
	EditTypeFolderVariable
)

type Model struct {
	parser          *postman.Parser
	executor        *http.Executor
	commandRegistry *CommandRegistry
	mode            ViewMode
	commandMode     bool
	commandInput    string
	commandHistory  []string
	historyIndex    int
	commandSuggestion string
	cursor          int
	items           []string
	currentItems    []postman.Item
	collection      *postman.Collection
	breadcrumb      []string
	width           int
	height          int
	statusMessage   string
	lastResponse    *http.Response
	currentInfoItem *postman.Item
	scrollOffset    int
	searchMode      bool
	searchQuery     string
	searchActive    bool
	filteredItems   []string
	filteredIndices []int
	allItems        []string
	allCurrentItems []postman.Item
	environment     *postman.Environment
	previousMode    ViewMode
	variables       []postman.VariableSource
	envVarCursor    int

	editType             EditType
	editRequest          *postman.Request
	editVariable         *postman.Variable
	editEnvVariable      *postman.EnvVariable
	editItemName         string
	editOriginalName     string
	editFieldCursor      int
	editFieldInput       string
	editFieldMode        bool
	modifiedItems        map[string]bool
	modifiedCollections  map[string]bool
	modifiedEnvironments map[string]bool
	modifiedRequests     map[string]*postman.Request
	editItemPath         []string
	editCollectionName   string
	editEnvironmentName  string
}

func NewModel(parser *postman.Parser) Model {
	return Model{
		parser:               parser,
		executor:             http.NewExecutor(),
		commandRegistry:      NewCommandRegistry(),
		mode:                 ModeCollections,
		commandMode:          false,
		commandInput:         "",
		commandHistory:       []string{},
		historyIndex:         -1,
		commandSuggestion:    "",
		cursor:               0,
		items:                []string{},
		currentItems:         []postman.Item{},
		breadcrumb:           []string{},
		statusMessage:        "Press : to enter command mode",
		modifiedItems:        make(map[string]bool),
		modifiedCollections:  make(map[string]bool),
		modifiedEnvironments: make(map[string]bool),
		modifiedRequests:     make(map[string]*postman.Request),
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
