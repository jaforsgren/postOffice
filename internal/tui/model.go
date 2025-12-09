package tui

import (
	"postOffice/internal/postman"

	tea "github.com/charmbracelet/bubbletea"
)

type ViewMode int

const (
	ModeCollections ViewMode = iota
	ModeRequests
)

type Model struct {
	parser        *postman.Parser
	mode          ViewMode
	commandMode   bool
	commandInput  string
	cursor        int
	items         []string
	currentItems  []postman.Item
	collection    *postman.Collection
	breadcrumb    []string
	width         int
	height        int
	statusMessage string
}

func NewModel(parser *postman.Parser) Model {
	return Model{
		parser:        parser,
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
