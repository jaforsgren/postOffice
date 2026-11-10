package tui

import (
	"strings"

	"postOffice/internal/postman"
)

const (
	requestPanelNotSentMessage = "Not sent yet: ctrl+r to send"
	requestPanelStatusMessage  = "Request info (ctrl+r send, s save response, y copy, ctrl+g editor, q close)"
)

// openRequestPanel shows a request's definition and its latest exchange in
// one view. It is the Response view, so resend, save, copy and open in
// editor all work there.
func (m Model) openRequestPanel(item postman.Item) Model {
	itemID := m.getRequestIdentifierByPath(m.collection.Info.Name, m.folderPathForCursor(), item.Name)
	m.lastResponse, m.lastTestResult = nil, nil
	if execution, exists := m.requestExecutions[itemID]; exists {
		m.lastResponse = execution.Response
		m.lastTestResult = execution.TestResult
	}
	m.lastExecutedItemID = itemID
	m.responsePanelItem = &item
	m.viewingSavedResponse = false
	m.scrollOffset = 0
	m.previousMode = m.mode
	m.mode = ModeResponse

	m.configureViewport(&m.responseViewport, strings.Join(m.responseViewLines(), "\n"))
	m.statusMessage = requestPanelStatusMessage
	return m
}

// responseViewLines is the Response view's content: the exchange, preceded
// by the request's definition when the view was opened on a request.
func (m Model) responseViewLines() []string {
	if m.responsePanelItem == nil {
		return m.buildResponseLines()
	}

	infoModel := m
	infoModel.currentInfoItem = m.responsePanelItem
	infoModel.breadcrumb = m.requestsViewFolderPath()
	lines := infoModel.unresolvedVariablesLines(m.responsePanelItem.Request, infoModel.breadcrumb)
	lines = append(lines, infoModel.buildItemDetailLines()...)
	lines = append(lines, "")
	if m.lastResponse == nil {
		return append(lines, subtleStyle.Render(requestPanelNotSentMessage))
	}
	return append(lines, m.buildResponseLines()...)
}
