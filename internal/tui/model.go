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
)

type Model struct {
	parser          *postman.Parser
	executor        *http.Executor
	mode            ViewMode
	commandMode     bool
	commandInput    string
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
}

func NewModel(parser *postman.Parser) Model {
	return Model{
		parser:        parser,
		executor:      http.NewExecutor(),
		mode:          ModeCollections,
		commandMode:   false,
		commandInput:  "",
		cursor:        0,
		items:         []string{},
		currentItems:  []postman.Item{},
		breadcrumb:    []string{},
		statusMessage: "Press : to enter command mode",
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
