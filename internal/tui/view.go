package tui

import (
	"github.com/charmbracelet/lipgloss"
)

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}

	var sections []string

	sections = append(sections, m.renderTopBar())
	sections = append(sections, m.renderContent()...)
	sections = append(sections, m.renderCommandBar())
	sections = append(sections, m.renderStatusBar())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m Model) renderContent() []string {
	metrics := m.calculateSplitLayout()

	switch m.mode {
	case ModeResponse:
		return []string{m.renderResponseView()}

	case ModeInfo:
		return []string{m.renderInfoView()}

	case ModeJSON:
		if m.jsonContent != "" {
			return []string{
				m.renderItemsList(metrics.mainHeight),
				m.renderJSONPopup(metrics.popupHeight),
			}
		}
		return []string{m.renderMainWindow()}

	case ModeEdit:
		metrics := m.calculateLayout()
		return []string{m.renderEditPopup(metrics.contentHeight)}

	case ModeVariables:
		return []string{m.renderVariablesView()}

	case ModeLog:
		return []string{m.renderLogsView()}
	case ModeFileBrowser:
		return []string{m.renderFileBrowser()}

	default:
		return []string{m.renderMainWindow()}
	}
}
