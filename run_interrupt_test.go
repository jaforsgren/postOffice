package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

const hangPath = "hang"

// interruptingServer answers okPath like apiServer and cancels the run when
// hangPath is requested, holding that request until the client gives up.
func interruptingServer(t *testing.T, cancel context.CancelFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/") == hangPath {
			cancel()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u1"}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestExecuteRun_InterruptedRunReportsAndPersistsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := interruptingServer(t, cancel)
	dir := t.TempDir()
	collectionPath := writeJSONFile(t, dir, "api.postman_collection.json", &postman.Collection{
		Info: postman.Info{Name: "API", Schema: "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
		Items: []postman.Item{
			apiRequest("Get User", okPath, passingTest, setsVariable),
			apiRequest("Hang", hangPath),
			apiRequest("Health", okPath, passingTest),
		},
	})
	envPath := writeJSONFile(t, dir, "local.postman_environment.json", postman.Environment{
		Name:   "Local",
		Values: []postman.EnvVariable{{Key: "base", Value: server.URL, Enabled: true}},
	})
	before, err := os.ReadFile(collectionPath)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	opts := executionOptions{environmentPath: envPath, output: outputJSON, persist: true}

	err = executeRunContext(ctx, &out, collectionPath, opts, workflow.NewCollectionWorkflow)

	if !errors.Is(err, errRunInterrupted) {
		t.Fatalf("executeRunContext() error = %v, want errRunInterrupted", err)
	}
	report := decodeReport(t, out.String())
	if report.Status != workflow.StatusStopped || report.Passed {
		t.Errorf("report status = %s, passed = %v, want a stopped, unpassed run", report.Status, report.Passed)
	}
	if after, _ := os.ReadFile(collectionPath); !bytes.Equal(before, after) {
		t.Error("expected the collection file untouched by an interrupted --persist run")
	}
}

func TestRunCLI_InterruptedRunExits130(t *testing.T) {
	interrupted := &cliCommand{name: "interrupted-test", run: func(io.Writer, []string) error { return errRunInterrupted }}
	original := cliCommands
	cliCommands = append(append([]*cliCommand(nil), original...), interrupted)
	t.Cleanup(func() { cliCommands = original })

	if code, _, _ := cli(t, interrupted.name); code != exitInterrupted {
		t.Errorf("exit code = %d, want %d", code, exitInterrupted)
	}
}
