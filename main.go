package main

import (
	"flag"
	"fmt"
	"os"
	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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

	model := tui.NewModel(parser)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("failed to run TUI: %w", err)
	}

	return nil
}
