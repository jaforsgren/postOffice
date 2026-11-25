package tui

import (
	"context"
	"errors"
	"os/exec"
	"postOffice/internal/history"
	"postOffice/internal/postman"
	"strings"
	"testing"
)

func TestShellTerminal_HandsTheCommandToTheEventLoop(t *testing.T) {
	terminal := newShellTerminal()
	cmd := exec.Command("true")
	runErr := make(chan error, 1)
	go func() { runErr <- terminal.Run(context.Background(), cmd) }()

	msg, ok := terminal.waitForHandOff()().(TerminalHandOffMsg)
	if !ok || msg.handOff.cmd != cmd {
		t.Fatalf("waitForHandOff() = %+v, want the script's command", msg)
	}
	wantErr := errors.New("exit status 1")
	msg.handOff.done <- wantErr

	if err := <-runErr; err != wantErr {
		t.Errorf("Run() = %v, want the error the event loop reported", err)
	}
}

func TestShellTerminal_CancelledWhileWaitingForTheTerminal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := newShellTerminal().Run(ctx, exec.Command("true"))

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() = %v, want context.Canceled", err)
	}
}

func TestNewItemEditMode_BashTemplate(t *testing.T) {
	m := NewModel(postman.NewParser(), history.NewStore("")).enterNewItemEditMode(requestTypeBash)

	if m.editRequest.Method != postman.MethodBash || m.editRequest.URL.Raw != postman.SchemeBash || m.editRequest.Body.Raw != bashRequestTemplate {
		t.Errorf("new request = %+v, want the bash template", m.editRequest)
	}
}

func TestBashInfo_ShowsWorkingDirEnvironmentScriptsAndResolvedVariables(t *testing.T) {
	m := Model{
		environment: &postman.Environment{Values: []postman.EnvVariable{{Key: "app", Value: "orders", Enabled: true}}},
		currentInfoItem: &postman.Item{
			Name: "deploy",
			Request: &postman.Request{
				Method: postman.MethodBash,
				URL:    postman.URL{Raw: postman.SchemeBash + "~/dev/{{app}}"},
				Header: []postman.Header{{Key: "APP", Value: "{{app}}"}},
				Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: "echo one\necho two\necho three\necho four\necho five\necho {{app}}"},
			},
			Events: []postman.Event{
				{Listen: "prerequest", Script: postman.Script{Exec: []string{"pm.environment.set('app', 'orders');"}}},
				{Listen: "test", Script: postman.Script{Exec: []string{"pm.test('ok', () => {});"}}},
			},
		},
	}

	joined := strings.Join(m.buildItemDetailLines(testTerminalWidth), "\n")

	for _, want := range []string{
		"Bash Script", "Working Directory:", "~/dev/{{app}}", "~/dev/orders",
		"Environment:", "APP: {{app}}",
		"Script:", "    echo five", "    echo {{app}}", "    echo orders",
		"Pre-Request Scripts:", "pm.environment.set", "pm.test('ok'",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "HTTP Request") || strings.Contains(joined, "...") {
		t.Errorf("expected the whole script as a bash item, got:\n%s", joined)
	}
}

func TestBashInfo_DefaultWorkingDirectory(t *testing.T) {
	m := Model{currentInfoItem: &postman.Item{Name: "plain", Request: &postman.Request{Method: postman.MethodBash, URL: postman.URL{Raw: postman.SchemeBash}}}}

	joined := strings.Join(m.buildItemDetailLines(testTerminalWidth), "\n")

	if !strings.Contains(joined, bashDefaultWorkingDirLabel) || !strings.Contains(joined, bashEmptyScriptLabel) {
		t.Errorf("expected the default directory and empty script labels in:\n%s", joined)
	}
}
