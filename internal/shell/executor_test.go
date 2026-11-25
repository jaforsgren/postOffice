package shell

import (
	"context"
	"errors"
	nethttp "net/http"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"strings"
	"testing"
	"time"
)

const promptScript = `read -r -p "Continue? [y/n] " answer
[[ "$answer" == [yY]* ]] || { echo "aborted"; exit 3; }
echo "confirmed"`

func bashRequest(url, body string, headers ...postman.Header) *postman.Request {
	return &postman.Request{
		Method: postman.MethodBash,
		URL:    postman.URL{Raw: url},
		Header: headers,
		Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: body},
	}
}

func TestExecuteContext_CapturesOutputOfSuccessfulScript(t *testing.T) {
	resp := NewExecutor(nil).ExecuteContext(context.Background(), bashRequest(postman.SchemeBash, "echo out; echo err >&2"), nil)

	if resp.Error != nil {
		t.Fatalf("Error = %v", resp.Error)
	}
	if resp.StatusCode != nethttp.StatusOK || resp.Status != "Exit 0" || resp.Headers[HeaderExitCode][0] != "0" {
		t.Errorf("status = %d %q %v, want 200 Exit 0", resp.StatusCode, resp.Status, resp.Headers)
	}
	if !strings.Contains(resp.Body, "out\n") || !strings.Contains(resp.Body, "err\n") {
		t.Errorf("Body = %q, want stdout and stderr", resp.Body)
	}
	if resp.RequestMethod != postman.MethodBash {
		t.Errorf("RequestMethod = %q", resp.RequestMethod)
	}
}

func TestExecuteContext_NonZeroExitIsAFailedResponse(t *testing.T) {
	resp := NewExecutor(nil).ExecuteContext(context.Background(), bashRequest(postman.SchemeBash, "echo nope; exit 7"), nil)

	if resp.Error != nil {
		t.Fatalf("Error = %v, want a response, not an error", resp.Error)
	}
	if resp.StatusCode != nethttp.StatusInternalServerError || resp.Status != "Exit 7" || resp.Headers[HeaderExitCode][0] != "7" {
		t.Errorf("status = %d %q %v, want 500 Exit 7", resp.StatusCode, resp.Status, resp.Headers)
	}
	if resp.Body != "nope\n" {
		t.Errorf("Body = %q", resp.Body)
	}
}

func TestExecuteContext_ResolvesVariablesAndExportsHeaders(t *testing.T) {
	variables := []postman.VariableSource{{Key: "name", Value: "world"}, {Key: "token", Value: "secret"}}
	req := bashRequest(postman.SchemeBash, `echo "hello {{name}} $TOKEN"`, postman.Header{Key: "TOKEN", Value: "{{token}}"})

	resp := NewExecutor(nil).ExecuteContext(context.Background(), req, variables)

	if resp.Body != "hello world secret\n" {
		t.Errorf("Body = %q, want resolved variable and exported header", resp.Body)
	}
	if resp.RequestBody != `echo "hello world $TOKEN"` || resp.RequestHeaders["TOKEN"] != "secret" {
		t.Errorf("recorded request = %q %v", resp.RequestBody, resp.RequestHeaders)
	}
}

func TestExecuteContext_RunsInTheURLsWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("here"), 0o600); err != nil {
		t.Fatal(err)
	}

	resp := NewExecutor(nil).ExecuteContext(context.Background(), bashRequest(postman.SchemeBash+dir, "cat marker.txt"), nil)

	if resp.Body != "here" {
		t.Errorf("Body = %q, want the marker file read from the working directory", resp.Body)
	}
}

func TestExecuteContext_PromptAnsweredOnTerminal(t *testing.T) {
	tests := []struct {
		answer     string
		wantStatus int
		wantBody   string
	}{
		{"y\n", nethttp.StatusOK, "confirmed"},
		{"n\n", nethttp.StatusInternalServerError, "aborted"},
	}
	for _, tt := range tests {
		t.Run(strings.TrimSpace(tt.answer), func(t *testing.T) {
			var shown strings.Builder
			terminal := NewStdioTerminal(strings.NewReader(tt.answer), &shown, &shown)

			resp := NewExecutor(terminal).ExecuteContext(context.Background(), bashRequest(postman.SchemeBash, promptScript), nil)

			if resp.StatusCode != tt.wantStatus || !strings.Contains(resp.Body, tt.wantBody) {
				t.Errorf("response = %d %q, want %d containing %q", resp.StatusCode, resp.Body, tt.wantStatus, tt.wantBody)
			}
			if !strings.Contains(shown.String(), tt.wantBody) {
				t.Errorf("terminal showed %q, want the script output", shown.String())
			}
		})
	}
}

func TestExecuteContext_PromptWithoutTerminalReadsEOF(t *testing.T) {
	resp := NewExecutor(nil).ExecuteContext(context.Background(), bashRequest(postman.SchemeBash, promptScript), nil)

	if resp.StatusCode != nethttp.StatusInternalServerError || !strings.Contains(resp.Body, "aborted") {
		t.Errorf("response = %d %q, want the prompt to fail", resp.StatusCode, resp.Body)
	}
}

func TestExecuteContext_CancelStopsTheScript(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()

	resp := NewExecutor(nil).ExecuteContext(ctx, bashRequest(postman.SchemeBash, "sleep 10"), nil)

	if !errors.Is(resp.Error, context.DeadlineExceeded) {
		t.Errorf("Error = %v, want the context error", resp.Error)
	}
	if time.Since(started) > waitDelay+time.Second {
		t.Errorf("took %v, want the script killed", time.Since(started))
	}
}

func TestStdioTerminal_CancelledBeforeRunStartsNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var shown strings.Builder

	resp := NewExecutor(NewStdioTerminal(strings.NewReader(""), &shown, &shown)).ExecuteContext(ctx, bashRequest(postman.SchemeBash, "echo ran"), nil)

	if resp.Error == nil || shown.Len() != 0 {
		t.Errorf("Error = %v, shown %q, want nothing run", resp.Error, shown.String())
	}
}
