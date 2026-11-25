package main

import (
	"flag"
	"fmt"
	"os"
	"postOffice/internal/history"
	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/tui"
	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	args := os.Args[1:]
	if !isTUIInvocation(args) {
		os.Exit(runCLI(args, os.Stdout, os.Stderr))
	}
	if err := runTUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(exitRunFailed)
	}
}

func runTUI() error {
	logPath := flag.String("log", "", "path to log file for debugging file operations")
	flag.Parse()

	if err := logger.Init(*logPath); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}
	defer logger.Close()

	parser := postman.NewParser()
	if err := parser.LoadState(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load previous state: %v\n", err)
	}

	model := tui.NewModel(parser, loadRequestHistory()).WithWorkflowRuns(loadWorkflowRuns())

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}

// loadRequestHistory falls back to an in-memory history when the history file
// cannot be located or read, so the TUI still starts.
func loadRequestHistory() *history.Store {
	historyPath, err := history.DefaultPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: request history will not be persisted: %v\n", err)
		return history.NewStore("")
	}

	requestHistory := history.NewStore(historyPath)
	if err := requestHistory.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load request history: %v\n", err)
	}
	return requestHistory
}

// loadWorkflowRuns falls back to keeping workflow runs in memory when the run
// store cannot be located or read, so the TUI still starts.
func loadWorkflowRuns() *workflow.RunStore {
	runsPath, err := workflow.DefaultRunStorePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: workflow runs will not be persisted: %v\n", err)
		return workflow.NewRunStore("")
	}

	runs := workflow.NewRunStore(runsPath)
	if err := runs.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to load workflow runs: %v\n", err)
	}
	return runs
}
