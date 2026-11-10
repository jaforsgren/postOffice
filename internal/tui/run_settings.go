package tui

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"

	tea "github.com/charmbracelet/bubbletea"
)

// :runopts subcommands and their values.
const (
	runOptStop       = "stop"
	runOptIterations = "iterations"
	runOptEnv        = "env"
	runOptData       = "data"
	runOptReset      = "reset"

	runOptOn           = "on"
	runOptOff          = "off"
	runOptActiveEnv    = "active"
	runOptAutoIterates = 0

	runOptsUsage = "Usage: :runopts [stop on|off] [iterations <n>|0] [env <name>|active] [data <file.csv|.json>|off] [reset]"
	runOptsSep   = " · "
)

var (
	errUnknownRunOption  = errors.New("unknown run option")
	errInvalidRunValue   = errors.New("invalid value")
	errUnknownRunEnvName = errors.New("environment not loaded")
)

// runSettings are the run options set with :runopts. They last for the
// session and apply to every workflow run started from the TUI, including
// folder runs (ctrl+e on a folder).
type runSettings struct {
	stopOnFailure bool
	// iterations is the explicit iteration count; 0 means one per data row (or one).
	iterations int
	// environmentName picks a loaded environment for runs; "" means the active one.
	environmentName string
	// dataPath is a CSV or JSON data file driving the iterations; "" means none.
	dataPath string
}

func (s runSettings) isDefault() bool {
	return s == runSettings{}
}

// describe summarises the non-default settings, e.g. "3 iterations · stop on failure".
func (s runSettings) describe() string {
	if s.isDefault() {
		return "defaults (1 iteration, continue on failure, active environment, no data)"
	}
	var parts []string
	if s.iterations > 0 {
		parts = append(parts, fmt.Sprintf("%d iterations", s.iterations))
	}
	if s.stopOnFailure {
		parts = append(parts, "stop on failure")
	}
	if s.environmentName != "" {
		parts = append(parts, "env: "+s.environmentName)
	}
	if s.dataPath != "" {
		parts = append(parts, "data: "+filepath.Base(s.dataPath))
	}
	return strings.Join(parts, runOptsSep)
}

// runConfig resolves the settings into what a run needs: the environment and
// the engine options. The data file is read here, at run start, so edits to
// it between runs are picked up.
func (m Model) runConfig() (*postman.Environment, workflow.ExecutionOptions, error) {
	settings := m.runSettings
	opts := workflow.ExecutionOptions{StopOnFailure: settings.stopOnFailure, Iterations: settings.iterations}

	environment := m.environment
	if settings.environmentName != "" {
		selected, ok := m.parser.GetEnvironment(settings.environmentName)
		if !ok {
			return nil, opts, fmt.Errorf("%w: %q", errUnknownRunEnvName, settings.environmentName)
		}
		environment = selected
	}

	if settings.dataPath != "" {
		data, err := workflow.LoadIterationData(settings.dataPath)
		if err != nil {
			return nil, opts, err
		}
		opts.Data = data
	}
	return environment, opts, nil
}

func handleRunOptsCommand(m Model, args []string) (Model, tea.Cmd) {
	if len(args) == 0 {
		m.statusMessage = "Run options: " + m.runSettings.describe()
		return m, nil
	}

	updated, err := m.applyRunOption(args[0])
	if err != nil {
		m.statusMessage = fmt.Sprintf("%v. %s", err, runOptsUsage)
		m.statusIsError = true
		return m, nil
	}
	m.runSettings = updated
	m.statusMessage = "Run options: " + updated.describe()
	return m, nil
}

// applyRunOption parses one "<option> <value>" argument string into updated settings.
func (m Model) applyRunOption(arg string) (runSettings, error) {
	settings := m.runSettings
	option, value, _ := strings.Cut(strings.TrimSpace(arg), " ")
	value = strings.TrimSpace(value)

	switch option {
	case runOptStop:
		switch value {
		case runOptOn:
			settings.stopOnFailure = true
		case runOptOff:
			settings.stopOnFailure = false
		default:
			return settings, fmt.Errorf("%w for %s: %q", errInvalidRunValue, runOptStop, value)
		}
	case runOptIterations:
		count, err := strconv.Atoi(value)
		if err != nil || count < runOptAutoIterates {
			return settings, fmt.Errorf("%w for %s: %q", errInvalidRunValue, runOptIterations, value)
		}
		settings.iterations = count
	case runOptEnv:
		switch {
		case value == runOptActiveEnv:
			settings.environmentName = ""
		case value == "":
			return settings, fmt.Errorf("%w for %s: missing name", errInvalidRunValue, runOptEnv)
		default:
			if _, ok := m.parser.GetEnvironment(value); !ok {
				return settings, fmt.Errorf("%w: %q", errUnknownRunEnvName, value)
			}
			settings.environmentName = value
		}
	case runOptData:
		switch value {
		case runOptOff:
			settings.dataPath = ""
		case "":
			return settings, fmt.Errorf("%w for %s: missing file", errInvalidRunValue, runOptData)
		default:
			path := expandPath(value)
			// Fail fast on an unreadable file; runConfig reads it again at run start.
			if _, err := workflow.LoadIterationData(path); err != nil {
				return settings, err
			}
			settings.dataPath = path
		}
	case runOptReset:
		settings = runSettings{}
	default:
		return settings, fmt.Errorf("%w: %q", errUnknownRunOption, option)
	}
	return settings, nil
}
