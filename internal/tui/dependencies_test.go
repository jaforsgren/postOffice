package tui

import (
	nethttp "net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"postOffice/internal/dependency"
	"postOffice/internal/history"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	depCollectionName = "Dependency Collection"
	depLoginPath      = "Auth/Login"
	depGetUserPath    = "Users/Get User"
	depLoginRoute     = "/login"
	depUserRoute      = "/user"
	depTokenVar       = "token"
	depMaxChainSteps  = 10
	depGraphWithLogin = `requests:
  Users/Get User:
    requires: [Auth/Login]
    produces: [userId]
  Auth/Login:
    produces: [token]
`
	setTokenScript = "pm.collectionVariables.set('token', 'abc');"
)

// recordingServer answers each route with the configured status and records the hit order.
type recordingServer struct {
	*httptest.Server
	mu       sync.Mutex
	hits     []string
	statuses map[string]int
}

func newRecordingServer(t *testing.T, statuses map[string]int) *recordingServer {
	rs := &recordingServer{statuses: statuses}
	rs.Server = httptest.NewServer(nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
		rs.mu.Lock()
		rs.hits = append(rs.hits, r.URL.Path)
		rs.mu.Unlock()
		status, ok := rs.statuses[r.URL.Path]
		if !ok {
			status = nethttp.StatusOK
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) recordedHits() []string {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]string{}, rs.hits...)
}

func dependencyTestCollection(serverURL string, loginEvents []postman.Event) *postman.Collection {
	request := func(method, route string) *postman.Request {
		return &postman.Request{Method: method, URL: postman.URL{Raw: serverURL + route}}
	}
	return &postman.Collection{
		Info: postman.Info{
			Name:   depCollectionName,
			Schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json",
		},
		Items: []postman.Item{
			{Name: "Auth", Items: []postman.Item{
				{Name: "Login", Request: request("POST", depLoginRoute), Events: loginEvents},
			}},
			{Name: "Users", Items: []postman.Item{
				{Name: "Get User", Request: request("GET", depUserRoute)},
			}},
		},
	}
}

func setTokenEvents() []postman.Event {
	return []postman.Event{{Listen: "test", Script: postman.Script{Type: "text/javascript", Exec: []string{setTokenScript}}}}
}

// newDependencyTestModel loads the collection from a temp file (so its dependency sidecar
// can be resolved), writes graphYAML next to it and opens the Users folder.
func newDependencyTestModel(t *testing.T, collection *postman.Collection, graphYAML string) (Model, string) {
	t.Helper()
	collectionPath := writeDependencyTestFiles(t, collection, graphYAML)

	parser := postman.NewParser()
	loaded, err := parser.LoadCollection(collectionPath)
	if err != nil {
		t.Fatal(err)
	}

	m := NewModel(parser, history.NewStore(""))
	m.collection = loaded
	m.environment = nil
	m.breadcrumb = nil
	m.mode = ModeRequests
	m = m.loadRequestsList()
	m = m.navigateInto(loaded.Items[1])
	m.cursor = 0
	return m, collectionPath
}

// writeDependencyTestFiles writes collection to a temp file and, when graphYAML
// is set, its dependency sidecar; it returns the collection path.
func writeDependencyTestFiles(t *testing.T, collection *postman.Collection, graphYAML string) string {
	t.Helper()
	collectionPath := writeJSONFile(t, "deps.postman_collection.json", collection)
	if graphYAML != "" {
		if err := os.WriteFile(dependency.FilePath(collectionPath), []byte(graphYAML), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return collectionPath
}

// drainCommands runs each returned command and feeds its message back into Update until none remain.
func drainCommands(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; cmd != nil; i++ {
		if i >= depMaxChainSteps {
			t.Fatal("command chain did not terminate")
		}
		updated, next := m.Update(cmd())
		m, cmd = updated.(Model), next
	}
	return m
}

func TestDependencyKeys_RegisteredInRequestsMode(t *testing.T) {
	m := createTestModel()
	for _, key := range []string{runWithPrerequisitesKey, editDependenciesKey} {
		if _, _, handled := m.commandRegistry.HandleKey(m, key); !handled {
			t.Errorf("expected %q to be handled in Requests mode", key)
		}
	}
}

func TestRunWithPrerequisites_RunsStalePrerequisitesFirst(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, setTokenEvents()), depGraphWithLogin)

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	if m.prerequisiteRun == nil {
		t.Fatalf("expected prerequisite run to start, status: %s", m.statusMessage)
	}
	m = drainCommands(t, m, cmd)

	if got, want := server.recordedHits(), []string{depLoginRoute, depUserRoute}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
	if m.prerequisiteRun != nil {
		t.Error("expected prerequisite run to be cleared after the target completes")
	}
	if !strings.Contains(m.statusMessage, "after 1 prerequisite") {
		t.Errorf("unexpected status: %s", m.statusMessage)
	}
}

func TestRunWithPrerequisites_FreshPrerequisiteIsSkipped(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, setTokenEvents()), depGraphWithLogin)

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)
	m, cmd = pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)

	want := []string{depLoginRoute, depUserRoute, depUserRoute}
	if got := server.recordedHits(); !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
}

func TestRunWithPrerequisites_FailingPrerequisiteStopsChain(t *testing.T) {
	server := newRecordingServer(t, map[string]int{depLoginRoute: nethttp.StatusInternalServerError})
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, setTokenEvents()), depGraphWithLogin)

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)

	if got, want := server.recordedHits(), []string{depLoginRoute}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
	if m.prerequisiteRun != nil {
		t.Error("expected prerequisite run to be cleared after a failure")
	}
	for _, fragment := range []string{`Prerequisite "Auth/Login" failed (status 500)`, `stopped before "Users/Get User"`} {
		if !strings.Contains(m.statusMessage, fragment) {
			t.Errorf("status %q missing %q", m.statusMessage, fragment)
		}
	}
}

func TestRunWithPrerequisites_UnsetProducedVariableStopsChain(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), depGraphWithLogin)

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)

	if got, want := server.recordedHits(), []string{depLoginRoute}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
	if !strings.Contains(m.statusMessage, `did not set `+depTokenVar) {
		t.Errorf("unexpected status: %s", m.statusMessage)
	}
}

func TestRunWithPrerequisites_NoDependenciesExecutesDirectly(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), "")

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	if m.prerequisiteRun != nil {
		t.Error("expected no prerequisite run without declared dependencies")
	}
	drainCommands(t, m, cmd)

	if got, want := server.recordedHits(), []string{depUserRoute}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v", got, want)
	}
}

func TestRunWithPrerequisites_InvalidGraphIsReported(t *testing.T) {
	tests := []struct {
		name      string
		graphYAML string
		want      string
	}{
		{
			name:      "cycle",
			graphYAML: "requests:\n  Users/Get User:\n    requires: [Auth/Login]\n  Auth/Login:\n    requires: [Users/Get User]\n",
			want:      "dependency cycle: Auth/Login → Users/Get User → Auth/Login",
		},
		{
			name:      "missing prerequisite",
			graphYAML: "requests:\n  Users/Get User:\n    requires: [Auth/Logout]\n",
			want:      `request "Auth/Logout" required by "Users/Get User" not found in collection`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRecordingServer(t, nil)
			m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), tt.graphYAML)

			m, cmd := pressKey(m, runes(runWithPrerequisitesKey))

			if cmd != nil {
				t.Error("expected no command for an invalid dependency graph")
			}
			if !strings.Contains(m.statusMessage, tt.want) {
				t.Errorf("status %q missing %q", m.statusMessage, tt.want)
			}
		})
	}
}

func TestBuildDependenciesSection_RendersTree(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), depGraphWithLogin)
	m.currentInfoItem = &m.currentItems[0]

	section := strings.Join(m.buildDependenciesSection(), "\n")

	for _, fragment := range []string{
		"Get User",
		dependencyTreeBranch + dependencyRequiresLabel + depLoginPath,
		prerequisiteStale,
		dependencyTreeLastBranch + dependencyProducesLabel + "userId",
		variableUnset,
	} {
		if !strings.Contains(section, fragment) {
			t.Errorf("section missing %q:\n%s", fragment, section)
		}
	}
}

func TestBuildDependenciesSection_EmptyWithoutDeclaration(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), "")
	m.currentInfoItem = &m.currentItems[0]

	if section := m.buildDependenciesSection(); len(section) != 0 {
		t.Errorf("expected no section, got %v", section)
	}
}

func TestOpenVimForDependencies_ScaffoldsSelectedRequest(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, collectionPath := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil), "")

	_, cmd := m.openVimForDependencies()
	if cmd == nil {
		t.Fatalf("expected editor command, status: %s", m.statusMessage)
	}

	graph, err := dependency.Load(dependency.FilePath(collectionPath))
	if err != nil {
		t.Fatal(err)
	}
	if _, declared := graph.Requests[depGetUserPath]; !declared {
		t.Errorf("expected scaffolded entry for %q, got %+v", depGetUserPath, graph.Requests)
	}
}

func TestApplyVimDependenciesEdit_ReportsInvalidGraph(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, nil),
		"requests:\n  Users/Get User:\n    requires: [Users/Get User]\n")

	m = m.applyVimDependenciesEdit(VimDependenciesEditCompleteMsg{ViewFolderPath: m.folderPathForCursor()})

	if !strings.Contains(m.statusMessage, "dependency cycle: Users/Get User → Users/Get User") {
		t.Errorf("unexpected status: %s", m.statusMessage)
	}
}

func TestRunWithPrerequisites_CombinedViewResolvesWithinOwningCollection(t *testing.T) {
	server := newRecordingServer(t, nil)
	pathA := writeDependencyTestFiles(t, dependencyTestCollection(server.URL, setTokenEvents()), depGraphWithLogin)
	pathB := writeJSONFile(t, "other.json", postman.Collection{Info: postman.Info{Name: combinedB}})

	parser := postman.NewParser()
	for _, path := range []string{pathA, pathB} {
		if _, err := parser.LoadCollection(path); err != nil {
			t.Fatal(err)
		}
	}
	parser.ToggleActiveCollection(depCollectionName)
	parser.ToggleActiveCollection(combinedB)
	m := NewModel(parser, history.NewStore(""))
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0
	m = m.handleSelection()
	if !m.isViewingMergedCollection() {
		t.Fatal("expected the combined view")
	}
	m = cursorOn(t, openFolder(t, openFolder(t, m, depCollectionName), "Users"), "Get User")

	m, cmd := pressKey(m, runes(runWithPrerequisitesKey))
	m = drainCommands(t, m, cmd)

	if got, want := server.recordedHits(), []string{depLoginRoute, depUserRoute}; !reflect.DeepEqual(got, want) {
		t.Errorf("hits = %v, want %v (status: %s)", got, want, m.statusMessage)
	}
}

func TestRunWithPrerequisites_CancelledPrerequisiteStopsChain(t *testing.T) {
	server := newRecordingServer(t, nil)
	m, _ := newDependencyTestModel(t, dependencyTestCollection(server.URL, setTokenEvents()), depGraphWithLogin)

	m, _ = pressKey(m, runes(runWithPrerequisitesKey))
	if m.prerequisiteRun == nil {
		t.Fatalf("expected prerequisite run to start, status: %s", m.statusMessage)
	}
	loginID := m.prerequisiteRun.currentStep().itemID
	updated, cmd := m.Update(RequestCompleteMsg{Cancelled: true, ItemID: loginID, ItemName: "Login", Response: &internalhttp.Response{}})
	m = updated.(Model)

	if cmd != nil || m.prerequisiteRun != nil {
		t.Error("expected the chain to stop after a cancelled prerequisite")
	}
	if !strings.Contains(m.statusMessage, `Cancelled "Auth/Login"; stopped before "Users/Get User"`) {
		t.Errorf("unexpected status: %s", m.statusMessage)
	}
}
