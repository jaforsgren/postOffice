package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

// editorBin returns the editor to use: $VISUAL → $EDITOR → nvim → vim.
func editorBin() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if v := os.Getenv("EDITOR"); v != "" {
		return v
	}
	if _, err := exec.LookPath("nvim"); err == nil {
		return "nvim"
	}
	return "vim"
}

// ── request editing via temp JSON ────────────────────────────────────────────

type vimEditPayload struct {
	Name    string           `json:"name"`
	Method  string           `json:"method"`
	URL     string           `json:"url"`
	Headers []postman.Header `json:"headers"`
	Body    string           `json:"body"`
}

// VimEditCompleteMsg is sent when the external editor exits after editing a request.
// FolderPath and OriginalName are stored explicitly so request names containing "/"
// never corrupt the lookup (e.g. "GET /api/users").
type VimEditCompleteMsg struct {
	TempFile     string
	ItemID       string
	FolderPath   []string
	OriginalName string
	// FromEditMode is true when vim was opened from ModeEdit, so the result
	// should update the in-progress edit state rather than the collection directly.
	FromEditMode bool
	Err          error
}

func (m Model) openVimForItem(item postman.Item) (Model, tea.Cmd) {
	if item.Request == nil {
		m.statusMessage = "No request data to edit"
		return m, nil
	}

	headers := item.Request.Header
	if headers == nil {
		headers = []postman.Header{}
	}

	payload := vimEditPayload{
		Name:    item.Name,
		Method:  item.Request.Method,
		URL:     item.Request.URL.Raw,
		Headers: headers,
	}
	if item.Request.Body != nil {
		payload.Body = item.Request.Body.Raw
	}

	itemID := m.getRequestIdentifier(item)
	folderPath := append([]string{}, m.breadcrumb...)
	return m, openVimEditorForRequest(payload, itemID, folderPath, item.Name, false)
}

func (m Model) openVimForEditState() (Model, tea.Cmd) {
	if m.editRequest == nil {
		m.statusMessage = "No request in edit state"
		return m, nil
	}

	headers := m.editRequest.Header
	if headers == nil {
		headers = []postman.Header{}
	}

	payload := vimEditPayload{
		Name:    m.editItemName,
		Method:  m.editRequest.Method,
		URL:     m.editRequest.URL.Raw,
		Headers: headers,
	}
	if m.editRequest.Body != nil {
		payload.Body = m.editRequest.Body.Raw
	}

	itemID := m.getRequestIdentifierByPath(m.editCollectionName, m.editItemPath, m.editOriginalName)
	folderPath := append([]string{}, m.editItemPath...)
	return m, openVimEditorForRequest(payload, itemID, folderPath, m.editOriginalName, true)
}

func openVimEditorForRequest(payload vimEditPayload, itemID string, folderPath []string, originalName string, fromEditMode bool) tea.Cmd {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return func() tea.Msg {
			return VimEditCompleteMsg{Err: fmt.Errorf("serialize request: %w", err)}
		}
	}

	tmpFile, err := os.CreateTemp("", "postoffice-edit-*.json")
	if err != nil {
		return func() tea.Msg {
			return VimEditCompleteMsg{Err: fmt.Errorf("create temp file: %w", err)}
		}
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return func() tea.Msg {
			return VimEditCompleteMsg{Err: fmt.Errorf("write temp file: %w", err)}
		}
	}
	tmpFile.Close()

	tempFilePath := tmpFile.Name()
	return tea.ExecProcess(exec.Command(editorBin(), tempFilePath), func(err error) tea.Msg {
		return VimEditCompleteMsg{
			TempFile:     tempFilePath,
			ItemID:       itemID,
			FolderPath:   folderPath,
			OriginalName: originalName,
			FromEditMode: fromEditMode,
			Err:          err,
		}
	})
}

func (m Model) applyVimEdit(msg VimEditCompleteMsg) Model {
	defer os.Remove(msg.TempFile)

	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}

	data, err := os.ReadFile(msg.TempFile)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to read edited file: %v", err)
		return m
	}

	var payload vimEditPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		m.statusMessage = fmt.Sprintf("Invalid JSON in edited file: %v", err)
		return m
	}

	if msg.FromEditMode && m.editRequest != nil {
		return m.applyVimEditToEditState(payload)
	}
	return m.applyVimEditToCollection(msg, payload)
}

func (m Model) applyVimEditToEditState(payload vimEditPayload) Model {
	m.editItemName = payload.Name
	m.editRequest.Method = payload.Method
	m.editRequest.URL.Raw = payload.URL
	m.editRequest.Header = payload.Headers

	if m.editRequest.Body == nil {
		m.editRequest.Body = &postman.Body{Mode: "raw"}
	}
	m.editRequest.Body.Raw = payload.Body

	m.statusMessage = fmt.Sprintf("Updated from editor: %s (use :w to save to file)", payload.Name)
	return m
}

func (m Model) applyVimEditToCollection(msg VimEditCompleteMsg, payload vimEditPayload) Model {
	newName := payload.Name
	if newName == "" {
		newName = msg.OriginalName
	}

	updatedReq := &postman.Request{
		Method: payload.Method,
		URL:    postman.URL{Raw: payload.URL},
		Header: payload.Headers,
	}
	if payload.Body != "" {
		updatedReq.Body = &postman.Body{Mode: "raw", Raw: payload.Body}
	}

	if !m.updateRequestInCollection(msg.FolderPath, msg.OriginalName, newName, updatedReq) {
		m.statusMessage = "Failed to update request in collection"
		return m
	}

	m.modifiedRequests[msg.ItemID] = updatedReq
	m.modifiedItems[msg.ItemID] = true
	if m.collection != nil {
		m.modifiedCollections[m.collection.Info.Name] = true
	}

	m = m.refreshCurrentView()
	m.statusMessage = fmt.Sprintf("Updated from editor: %s (use :w to save to file)", newName)
	return m
}

// ── environment editing (open the actual file) ────────────────────────────────

// VimEnvEditCompleteMsg is sent when the editor exits after editing an environment file.
type VimEnvEditCompleteMsg struct {
	EnvName string
	Err     error
}

func (m Model) openVimForEnvironment() (Model, tea.Cmd) {
	envName := ""
	if m.mode == ModeEnvironments && m.cursor < len(m.items) {
		envName = m.items[m.cursor]
	} else if m.environment != nil {
		envName = m.environment.Name
	}

	if envName == "" {
		m.statusMessage = "No environment selected"
		return m, nil
	}

	path, exists := m.parser.GetEnvironmentPath(envName)
	if !exists {
		m.statusMessage = fmt.Sprintf("Path not found for environment: %s", envName)
		return m, nil
	}

	return m, tea.ExecProcess(exec.Command(editorBin(), path), func(err error) tea.Msg {
		return VimEnvEditCompleteMsg{EnvName: envName, Err: err}
	})
}

func (m Model) applyVimEnvEdit(msg VimEnvEditCompleteMsg) Model {
	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}

	path, exists := m.parser.GetEnvironmentPath(msg.EnvName)
	if !exists {
		m.statusMessage = fmt.Sprintf("Cannot reload environment: path not found for %s", msg.EnvName)
		return m
	}

	env, err := m.parser.LoadEnvironment(path)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to reload environment: %v", err)
		return m
	}

	if m.environment != nil && m.environment.Name == msg.EnvName {
		m.environment = env
	}

	m.statusMessage = fmt.Sprintf("Reloaded environment: %s", env.Name)
	return m
}

// ── workflow editing (open the actual file) ───────────────────────────────────

// VimWorkflowEditCompleteMsg is sent when the editor exits after editing a workflow file.
type VimWorkflowEditCompleteMsg struct {
	WorkflowID     string
	CollectionPath string
	Err            error
}

func (m Model) openVimForWorkflow() (Model, tea.Cmd) {
	var wf *workflow.Workflow
	if m.mode == ModeWorkflows && m.workflowCursor < len(m.workflows) {
		wf = m.workflows[m.workflowCursor]
	} else if m.activeWorkflow != nil {
		wf = m.activeWorkflow
	}

	if wf == nil {
		m.statusMessage = "No workflow selected"
		return m, nil
	}

	if m.collection == nil {
		m.statusMessage = "No collection loaded"
		return m, nil
	}

	collectionPath, exists := m.parser.GetCollectionPath(m.collection.Info.Name)
	if !exists {
		m.statusMessage = "Collection path not found"
		return m, nil
	}

	wfPath := workflow.WorkflowPath(collectionPath, wf.ID)
	return m, tea.ExecProcess(exec.Command(editorBin(), wfPath), func(err error) tea.Msg {
		return VimWorkflowEditCompleteMsg{
			WorkflowID:     wf.ID,
			CollectionPath: collectionPath,
			Err:            err,
		}
	})
}

func (m Model) applyVimWorkflowEdit(msg VimWorkflowEditCompleteMsg) Model {
	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}

	wfPath := workflow.WorkflowPath(msg.CollectionPath, msg.WorkflowID)
	wf, err := workflow.LoadWorkflow(wfPath)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to reload workflow: %v", err)
		return m
	}

	for i, w := range m.workflows {
		if w.ID == msg.WorkflowID {
			m.workflows[i] = wf
			break
		}
	}

	if m.activeWorkflow != nil && m.activeWorkflow.ID == msg.WorkflowID {
		m.activeWorkflow = wf
	}

	m.statusMessage = fmt.Sprintf("Reloaded workflow: %s", wf.Name)
	return m
}

// ── bulk level editing (open all items at current folder depth as JSON) ───────

// VimBulkEditCompleteMsg is sent when the editor exits after editing a folder's item array.
type VimBulkEditCompleteMsg struct {
	TempFile   string
	FolderPath []string
	Err        error
}

func (m Model) openVimForCurrentLevel() (Model, tea.Cmd) {
	if m.collection == nil {
		m.statusMessage = "No collection loaded"
		return m, nil
	}
	folderPath := append([]string{}, m.breadcrumb...)
	items := traverseToDepth(m.collection.Items, folderPath)
	if items == nil {
		m.statusMessage = "Cannot resolve current folder in collection"
		return m, nil
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to serialize items: %v", err)
		return m, nil
	}
	tmpFile, err := os.CreateTemp("", "postoffice-bulk-*.json")
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to create temp file: %v", err)
		return m, nil
	}
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		m.statusMessage = fmt.Sprintf("Failed to write temp file: %v", err)
		return m, nil
	}
	tmpFile.Close()
	tempFilePath := tmpFile.Name()
	return m, tea.ExecProcess(exec.Command(editorBin(), tempFilePath), func(err error) tea.Msg {
		return VimBulkEditCompleteMsg{
			TempFile:   tempFilePath,
			FolderPath: folderPath,
			Err:        err,
		}
	})
}

func (m Model) applyVimBulkEdit(msg VimBulkEditCompleteMsg) Model {
	defer os.Remove(msg.TempFile)

	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}

	data, err := os.ReadFile(msg.TempFile)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to read edited file: %v", err)
		return m
	}

	var items []postman.Item
	if err := json.Unmarshal(data, &items); err != nil {
		m.statusMessage = fmt.Sprintf("Invalid JSON in edited file: %v", err)
		return m
	}

	target := traverseToDepthPtr(&m.collection.Items, msg.FolderPath)
	if target == nil {
		m.statusMessage = "Cannot resolve folder path in collection"
		return m
	}
	*target = items

	m.modifiedCollections[m.collection.Info.Name] = true
	m = m.refreshCurrentView()

	depth := "root"
	if len(msg.FolderPath) > 0 {
		depth = strings.Join(msg.FolderPath, "/")
	}
	m.statusMessage = fmt.Sprintf("Updated items at %s (use :w to save)", depth)
	return m
}

// ── collection file editing (open the actual collection JSON file) ────────────

// VimCollectionEditCompleteMsg is sent when the editor exits after editing a collection file.
type VimCollectionEditCompleteMsg struct {
	CollectionName string
	FilePath       string
	Err            error
}

func (m Model) openVimForCollection() (Model, tea.Cmd) {
	if m.cursor >= len(m.items) {
		m.statusMessage = "No collection selected"
		return m, nil
	}
	collectionName := m.items[m.cursor]
	path, exists := m.parser.GetCollectionPath(collectionName)
	if !exists {
		m.statusMessage = fmt.Sprintf("Path not found for collection: %s", collectionName)
		return m, nil
	}
	return m, tea.ExecProcess(exec.Command(editorBin(), path), func(err error) tea.Msg {
		return VimCollectionEditCompleteMsg{
			CollectionName: collectionName,
			FilePath:       path,
			Err:            err,
		}
	})
}

func (m Model) applyVimCollectionEdit(msg VimCollectionEditCompleteMsg) Model {
	if msg.Err != nil {
		m.statusMessage = fmt.Sprintf("Editor error: %v", msg.Err)
		return m
	}

	newCollection, err := m.parser.LoadCollection(msg.FilePath)
	if err != nil {
		m.statusMessage = fmt.Sprintf("Failed to reload collection: %v", err)
		return m
	}

	if m.collection != nil && m.collection.Info.Name == msg.CollectionName {
		m.collection = newCollection
		m = m.refreshCurrentView()
	}

	m.statusMessage = fmt.Sprintf("Reloaded collection: %s", newCollection.Info.Name)
	return m
}
