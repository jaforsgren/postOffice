package tui

import (
	"context"
	"os"
	"os/exec"
	"postOffice/internal/shell"

	tea "github.com/charmbracelet/bubbletea"
)

// terminalHandOff asks the event loop to give the terminal to a bash script.
type terminalHandOff struct {
	cmd  *exec.Cmd
	done chan error
}

// shellTerminal runs bash scripts in the user's terminal, as $EDITOR is: the
// script's goroutine hands its command to the event loop, which suspends the
// TUI with tea.ExecProcess until the script exits. Only the event loop may
// release the terminal, hence the hand-off.
type shellTerminal struct {
	handOffs chan terminalHandOff
}

func newShellTerminal() *shellTerminal {
	return &shellTerminal{handOffs: make(chan terminalHandOff)}
}

func (t *shellTerminal) Run(ctx context.Context, cmd *exec.Cmd) error {
	// bubbletea connects stdin to its own input, which is still unset here.
	shell.ShowOutput(cmd, os.Stdout, os.Stderr)
	handOff := terminalHandOff{cmd: cmd, done: make(chan error, 1)}
	select {
	case t.handOffs <- handOff:
	case <-ctx.Done():
		return ctx.Err()
	}
	return <-handOff.done
}

// TerminalHandOffMsg carries a script waiting for the terminal to the event loop.
type TerminalHandOffMsg struct {
	handOff terminalHandOff
}

// TerminalReleasedMsg is sent once a script has given the terminal back.
type TerminalReleasedMsg struct{}

func (t *shellTerminal) waitForHandOff() tea.Cmd {
	return func() tea.Msg {
		return TerminalHandOffMsg{handOff: <-t.handOffs}
	}
}

func (m Model) handleTerminalHandOff(msg TerminalHandOffMsg) (tea.Model, tea.Cmd) {
	done := msg.handOff.done
	return m, tea.ExecProcess(msg.handOff.cmd, func(err error) tea.Msg {
		done <- err
		return TerminalReleasedMsg{}
	})
}

// handleTerminalReleased waits for the next script once the terminal is
// back, so scripts run one at a time.
func (m Model) handleTerminalReleased() (tea.Model, tea.Cmd) {
	return m, m.terminal.waitForHandOff()
}

// requestTypeBash is the new-request choice for a bash script.
const requestTypeBash = "Bash"

// bashRequestTemplate shows a new bash request how to prompt.
const bashRequestTemplate = `read -r -p "Continue? [y/n] " answer
[[ "$answer" == [yY]* ]] || exit 1
echo "done"`

const (
	bashDefaultWorkingDirLabel = "(PostOffice's current directory)"
	bashEmptyScriptLabel       = "(empty)"
)
