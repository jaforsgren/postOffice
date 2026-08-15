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
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-m", "--merge":
			return runMerge(os.Args[2:])
		case "alias":
			return runAlias(os.Args[2:])
		}
	}

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
