package tui

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"postOffice/internal/http"
	"postOffice/internal/postman"
)

// yankPrefixKey starts a vim-style two-key yank: the next key picks what is copied.
const yankPrefixKey = "y"

const (
	yankAllKey     = "a"
	yankBodyKey    = "b"
	yankRequestKey = "r"
	yankURLKey     = "y"
)

// openRequestReportKey opens the request report in the editor from the read-only views.
const openRequestReportKey = "ctrl+g"

const yankPendingMessage = "yank: a request+response · b body · r curl/grpcurl · y url"

var (
	errYankNoRequest  = errors.New("select a request to copy")
	errYankNoResponse = errors.New("no response yet: send the request first")
)

var errNothingToOpen = errors.New("nothing to open: select a request")

// yankTarget is what a yank copies from. request is what yr/yy copy: the
// request as sent when a response view is open, else the selected request
// resolved with its variables. definition is the collection item, when known.
type yankTarget struct {
	request    postman.Item
	vars       []postman.VariableSource
	definition *postman.Item
	response   *http.Response
	model      Model
}

func (m Model) currentYankTarget() (yankTarget, error) {
	switch m.mode {
	case ModeResponse:
		if m.lastResponse == nil {
			if m.responsePanelItem != nil {
				return m.definitionYankTarget(*m.responsePanelItem), nil
			}
			return yankTarget{}, errYankNoResponse
		}
		return yankTarget{
			request:    sentRequestItem(m.lastResponse),
			response:   m.lastResponse,
			definition: m.responsePanelItem,
			model:      m,
		}, nil
	case ModeInfo:
		showsRequestInfo := m.previousMode == ModeRequests || m.previousMode == ModeWorkflowDetail
		if !showsRequestInfo || m.currentInfoItem == nil || !m.currentInfoItem.IsRequest() {
			return yankTarget{}, errYankNoRequest
		}
		return m.definitionYankTarget(*m.currentInfoItem), nil
	}

	item, ok := m.selectedItem()
	if !ok || !item.IsRequest() {
		return yankTarget{}, errYankNoRequest
	}
	return m.definitionYankTarget(item), nil
}

// definitionYankTarget targets a collection request and its latest response.
func (m Model) definitionYankTarget(item postman.Item) yankTarget {
	target := yankTarget{
		request:    item,
		vars:       m.variablesAt(m.requestsViewFolderPath()),
		definition: &item,
		model:      m,
	}
	if execution, exists := m.requestExecutions[m.requestsViewItemID(item)]; exists && execution.Response != nil {
		target.response = execution.Response
		target.model.lastResponse = execution.Response
		target.model.lastTestResult = execution.TestResult
	}
	return target
}

// requestsViewFolderPath is the cursor's folder path in the requests view,
// also from the Info and Response views opened on top of it.
func (m Model) requestsViewFolderPath() []string {
	requestsView := m
	requestsView.mode = ModeRequests
	return requestsView.folderPathForCursor()
}

func (m Model) requestsViewItemID(item postman.Item) string {
	return m.getRequestIdentifierByPath(m.collection.Info.Name, m.requestsViewFolderPath(), item.Name)
}

// sentRequestItem rebuilds the request a response records as having been sent.
func sentRequestItem(response *http.Response) postman.Item {
	request := &postman.Request{
		Method: response.RequestMethod,
		URL:    postman.URL{Raw: response.RequestURL},
	}
	for _, key := range slices.Sorted(maps.Keys(response.RequestHeaders)) {
		request.Header = append(request.Header, postman.Header{Key: key, Value: response.RequestHeaders[key]})
	}
	if response.RequestBody != "" {
		request.Body = &postman.Body{Mode: "raw", Raw: response.RequestBody}
	}
	return postman.Item{Request: request}
}

func (t yankTarget) url() string {
	return postman.ResolveVariables(t.request.Request.URL.Raw, t.vars)
}

func (t yankTarget) curl() (string, string) {
	if t.request.IsGRPC() {
		return buildGRPCCurlCommand(t.request, t.vars), "grpcurl"
	}
	return buildCurlCommand(t.request.Request, t.vars), "curl"
}

// exchangeReport is the request as sent, the response, test results and
// console logs as plain text, as the response view shows them.
func (t yankTarget) exchangeReport() string {
	return plainText(t.model.buildResponseLines())
}

func plainText(lines []string) string {
	return ansi.Strip(strings.Join(lines, "\n")) + "\n"
}

// yankText returns the text the yank key copies and a label for the status line.
func (t yankTarget) yankText(key string) (string, string, error) {
	switch key {
	case yankURLKey:
		return t.url(), "URL", nil
	case yankRequestKey:
		text, kind := t.curl()
		return text, kind, nil
	case yankBodyKey:
		if t.response == nil {
			return "", "", errYankNoResponse
		}
		return t.response.Body, "response body", nil
	case yankAllKey:
		if t.response == nil {
			return "", "", errYankNoResponse
		}
		return t.exchangeReport(), "request and response", nil
	}
	return "", "", fmt.Errorf("unknown yank %q (%s)", yankPrefixKey+key, yankPendingMessage)
}

func handleYankPrefixKey(m Model) (Model, tea.Cmd) {
	m.pendingYank = true
	m.statusMessage = yankPendingMessage
	return m, nil
}

// handleYankKey completes a yank started by yankPrefixKey; esc cancels it.
func (m Model) handleYankKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.pendingYank = false
	key := msg.String()
	if key == "esc" {
		m.statusMessage = ""
		return m, nil
	}
	return m.yank(key), nil
}

func handleCopyCurlKey(m Model) (Model, tea.Cmd) {
	return m.yank(yankRequestKey), nil
}

func (m Model) yank(key string) Model {
	target, err := m.currentYankTarget()
	if err != nil {
		return m.copyFailed(err)
	}
	text, label, err := target.yankText(key)
	if err != nil {
		return m.copyFailed(err)
	}
	if err := clipboard.WriteAll(text); err != nil {
		return m.copyFailed(err)
	}
	m.statusMessage = fmt.Sprintf("Copied %s to clipboard", label)
	return m
}

func (m Model) copyFailed(err error) Model {
	m.statusMessage = fmt.Sprintf("Copy failed: %v", err)
	m.statusIsError = true
	return m
}

// VimRequestReportCompleteMsg is sent when the editor showing a request report exits.
type VimRequestReportCompleteMsg struct {
	TempFile string
	Err      error
}

// handleOpenRequestReportKey shows the request info, the exchange and its
// logs as Markdown in the editor, for searching and copying through large
// bodies. It is a snapshot: edits are discarded.
func handleOpenRequestReportKey(m Model) (Model, tea.Cmd) {
	target, err := m.currentYankTarget()
	if err != nil {
		m.statusMessage = err.Error()
		return m, nil
	}
	report, err := target.markdownReport()
	if err != nil {
		m.statusMessage = err.Error()
		return m, nil
	}

	tempFilePath, err := writeTempFile(requestReportFilePattern, report)
	if err != nil {
		m.statusMessage = err.Error()
		return m, nil
	}
	return m, tea.ExecProcess(exec.Command(editorBin(), tempFilePath), func(err error) tea.Msg {
		return VimRequestReportCompleteMsg{TempFile: tempFilePath, Err: err}
	})
}

func (m Model) applyVimRequestReport(msg VimRequestReportCompleteMsg) Model {
	os.Remove(msg.TempFile)
	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}
	m.statusMessage = "Closed request report"
	return m
}
