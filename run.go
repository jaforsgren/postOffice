package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"postOffice/internal/engine"
	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/shell"
	"postOffice/internal/workflow"

	"github.com/charmbracelet/x/term"
)

const reportFilePermissions = 0644

var (
	folderFlag  = flagSpec{names: []string{"--folder"}, takesValue: true}
	outputFlag  = flagSpec{names: []string{"--output"}, takesValue: true}
	reportFlag  = flagSpec{names: []string{"--report"}, takesValue: true}
	persistFlag = flagSpec{names: []string{"--persist"}}

	iterationsFlag = flagSpec{names: []string{"--iterations", "-n"}, takesValue: true}
	bailFlag       = flagSpec{names: []string{"--bail"}}
	dataFlag       = flagSpec{names: []string{"--data", "-d"}, takesValue: true}
)

// executionFlags are accepted by every command that sends requests, so they
// apply to `run` and `workflow run` alike.
var executionFlags = []flagSpec{environmentFlag, outputFlag, reportFlag, persistFlag, iterationsFlag, bailFlag, dataFlag}

// errPersistUnsupported is returned when --persist would overwrite a file that is not a Postman collection.
var errPersistUnsupported = errors.New("--persist only works with Postman collection files")

// executionOptions controls how a headless run executes and reports.
type executionOptions struct {
	environmentPath string
	output          outputFormat
	reportPath      string
	persist         bool
	// runner holds stop-on-failure and the iteration count; Data is loaded
	// from dataPath when the run starts, so a bad file is an input error.
	runner   workflow.ExecutionOptions
	dataPath string
}

func parseExecutionOptions(parsed parsedArgs) (executionOptions, error) {
	opts := executionOptions{
		environmentPath: parsed.value(environmentFlag),
		output:          outputText,
		reportPath:      parsed.value(reportFlag),
		persist:         parsed.has(persistFlag),
		runner:          workflow.ExecutionOptions{StopOnFailure: parsed.has(bailFlag)},
		dataPath:        parsed.value(dataFlag),
	}
	if parsed.has(iterationsFlag) {
		iterations, err := strconv.Atoi(parsed.value(iterationsFlag))
		if err != nil {
			return executionOptions{}, usageErrorf("--iterations must be a whole number, got %q", parsed.value(iterationsFlag))
		}
		opts.runner.Iterations = iterations
		if err := opts.runner.Validate(); err != nil {
			return executionOptions{}, usageErrorf("--iterations: %v", err)
		}
	}
	if parsed.has(outputFlag) {
		opts.output = outputFormat(parsed.value(outputFlag))
		if _, isMachine := machineWriters[opts.output]; !isMachine && opts.output != outputText {
			return executionOptions{}, usageErrorf("--output must be %s, %s or %s, got %q", outputText, outputJSON, outputJUnit, opts.output)
		}
	}
	if opts.reportPath != "" && opts.output == outputText {
		return executionOptions{}, usageErrorf("--report needs --output %s or --output %s", outputJSON, outputJUnit)
	}
	return opts, nil
}

// runOptions is a parsed `run` command line.
// workflowBuilder makes the workflow a run executes from the loaded collection.
type workflowBuilder func(*postman.Collection) (*workflow.Workflow, error)

type runOptions struct {
	collectionPath string
	// requestPath ("Folder/Sub/Request") runs one request; otherwise folderPath
	// runs a folder, and an empty folderPath runs the whole collection.
	requestPath string
	folderPath  []string
	execution   executionOptions
}

func parseRunArgs(args []string) (runOptions, error) {
	parsed, err := parseFlags(args, append([]flagSpec{folderFlag}, executionFlags...)...)
	if err != nil {
		return runOptions{}, err
	}

	var opts runOptions
	switch len(parsed.positional) {
	case 0:
	case 1:
		opts.collectionPath = parsed.positional[0]
	case 2:
		opts.collectionPath, opts.requestPath = parsed.positional[0], parsed.positional[1]
	default:
		return runOptions{}, usageErrorf("run takes a collection and optionally one request path, got %d arguments", len(parsed.positional))
	}

	if parsed.has(folderFlag) {
		if opts.requestPath != "" {
			return runOptions{}, usageErrorf("give either a request path or --folder, not both")
		}
		folder := parsed.value(folderFlag)
		if folder == "" {
			return runOptions{}, usageErrorf("--folder needs a folder path such as \"Users/Admin\"")
		}
		opts.folderPath = postman.SplitItemPath(folder)
	}

	opts.execution, err = parseExecutionOptions(parsed)
	return opts, err
}

var runCommand = &cliCommand{
	name:      "run",
	summary:   "Run a request, a folder or a whole collection with its scripts",
	aliasable: true,
	usage: `Usage:
  postOffice run <collection> "<Folder/Sub/Request>" [options]
  postOffice run <collection> --folder "<Folder/Sub>" [options]
  postOffice run <collection> [options]
  postOffice run ["<Folder/Sub/Request>"] [options]

<collection> is a collection file, or the name of a loaded collection (see
"postOffice ls"): its own name or its file name, e.g. SAPI for
SAPI.postman_collection.json, ignoring case. Without it, or when the only
argument isn't a collection, the selected collection (see "postOffice use")
runs, and the argument is a request path in it.

Runs one request, every request in a folder, or every request in the
collection, with collection → folder → request pre-request and test scripts.
Folder and collection runs go in collection order and follow
pm.execution.setNextRequest and skipRequest, like Postman's Collection Runner.
Request paths are those listed by "postOffice ls request". A "/" inside a
name is written "\/" and a "\" as "\\", e.g. 'NEGATIVE/unknown policy 403\/404'.

Options:
  -e, --environment <env>        environment file or loaded environment name
                                 to resolve variables from (default: the
                                 selected one)
  --folder <Folder/Sub>          run every request in this folder
  --output text|json|junit       report format (default text)
  --report <file>                write the json/junit report to a file and
                                 print the text report
  --persist                      save script variable changes back to the
                                 collection and environment files (off by default)
  -n, --iterations <n>           run n times (default: one per --data row, else 1)
  -d, --data <file.csv|json>     one iteration per row; values are available as
                                 {{key}} and pm.iterationData
  --bail                         stop at the first failed request, test or script

Exit codes: 0 all passed, 1 a request, test or script failed, 2 usage or input error,
130 interrupted (ctrl+c stops the run; variables are not persisted).`,
	run: func(out io.Writer, args []string) error {
		opts, err := parseRunArgs(args)
		if err != nil {
			return err
		}
		if opts, err = withSelectedCollection(opts); err != nil {
			return err
		}
		return executeRun(out, opts.collectionPath, opts.execution, func(collection *postman.Collection) (*workflow.Workflow, error) {
			switch {
			case opts.requestPath != "":
				return workflow.NewRequestWorkflow(collection, opts.requestPath)
			case len(opts.folderPath) > 0:
				return workflow.NewFolderPathWorkflow(collection, opts.folderPath)
			default:
				return workflow.NewCollectionWorkflow(collection)
			}
		})
	},
}

// executeRun loads the collection and environment (each a file path or a
// loaded one's name; the environment defaults to the selected one), runs the workflow buildWorkflow makes from the
// collection, and reports the result. SIGINT or SIGTERM stops the run.
func executeRun(out io.Writer, collectionRef string, opts executionOptions, buildWorkflow workflowBuilder) error {
	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	return executeRunContext(ctx, out, collectionRef, opts, buildWorkflow)
}

// executeRunContext is executeRun stopped by cancelling ctx instead of a
// signal. A stopped run reports how far it got and persists nothing.
func executeRunContext(ctx context.Context, out io.Writer, collectionRef string, opts executionOptions, buildWorkflow workflowBuilder) error {
	opts = withSelectedEnvironment(opts)
	parser, collection, environment, err := loadCollectionAndEnvironment(collectionRef, opts.environmentPath)
	if err != nil {
		return err
	}
	if opts.persist {
		collectionPath, _ := parser.GetCollectionPath(collection.Info.Name)
		if err := requirePostmanCollection(collectionPath); err != nil {
			return err
		}
	}

	runnerOptions := opts.runner
	if opts.dataPath != "" {
		if runnerOptions.Data, err = workflow.LoadIterationData(opts.dataPath); err != nil {
			return err
		}
	}

	wf, err := buildWorkflow(collection)
	if err != nil {
		return err
	}

	styles := newTextStyles(out)
	var onChange func(workflow.ExecutionState)
	if opts.writesText() {
		writeTextHeader(out, wf.Name, collection.Info.Name, environmentName(environment))
		onChange = newStepStream(out, styles).onChange
	}

	state := workflow.NewRunner(collection, environment, engine.NewWithTerminal(internalhttp.NewExecutor(), runTerminal())).Run(ctx, wf, runnerOptions, onChange)
	report := newRunReport(state, collection.Info.Name, environmentName(environment))
	if err := writeRunOutput(out, styles, report, opts); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return errRunInterrupted
	}

	if opts.persist {
		if err := persistVariables(parser, collection, environment); err != nil {
			return err
		}
	}

	if !report.Passed {
		return errRunFailed
	}
	return nil
}

// loadCollectionAndEnvironment loads the collection and, when environmentRef
// is set, the environment. Each is a file path or a loaded one's name.
func loadCollectionAndEnvironment(collectionRef, environmentRef string) (*postman.Parser, *postman.Collection, *postman.Environment, error) {
	collectionPath, err := resolveCollectionRef(collectionRef)
	if err != nil {
		return nil, nil, nil, err
	}
	parser := postman.NewParser()
	collection, err := parser.LoadCollection(collectionPath)
	if err != nil {
		return nil, nil, nil, err
	}
	if environmentRef == "" {
		return parser, collection, nil, nil
	}
	environmentPath, err := resolveEnvironmentRef(environmentRef)
	if err != nil {
		return nil, nil, nil, err
	}
	environment, err := parser.LoadEnvironment(environmentPath)
	if err != nil {
		return nil, nil, nil, err
	}
	return parser, collection, environment, nil
}

func environmentName(environment *postman.Environment) string {
	if environment == nil {
		return ""
	}
	return environment.Name
}

func requirePostmanCollection(path string) error {
	expanded, err := postman.ExpandPath(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(expanded)
	if err != nil {
		return fmt.Errorf("failed to read collection file: %w", err)
	}
	if postman.DetectFormat(data) != postman.FormatPostman {
		return fmt.Errorf("%w: %s", errPersistUnsupported, path)
	}
	return nil
}

// writesText reports whether the text report goes to out: with text output,
// or next to a --report file.
func (opts executionOptions) writesText() bool {
	_, isMachine := machineWriters[opts.output]
	return !isMachine || opts.reportPath != ""
}

// writeRunOutput finishes the report in the chosen format. The text report's
// steps were already streamed while the run went, so only its summary is
// left. With --report the machine-readable report goes to that file and the
// text report to out.
func writeRunOutput(out io.Writer, styles textStyles, report runReport, opts executionOptions) error {
	writeMachine, isMachine := machineWriters[opts.output]
	if !isMachine {
		writeTextSummary(out, styles, report)
		return nil
	}
	if opts.reportPath == "" {
		return writeMachine(out, report)
	}

	file, err := os.OpenFile(opts.reportPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, reportFilePermissions)
	if err != nil {
		return fmt.Errorf("failed to create report file: %w", err)
	}
	if err := writeMachine(file, report); err != nil {
		file.Close()
		return fmt.Errorf("failed to write report file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}
	writeTextSummary(out, styles, report)
	return nil
}

func persistVariables(parser *postman.Parser, collection *postman.Collection, environment *postman.Environment) error {
	if err := parser.SaveCollection(collection.Info.Name); err != nil {
		return err
	}
	if environment == nil {
		return nil
	}
	return parser.SaveEnvironment(environment.Name)
}

// withSelectedCollection runs the selected collection when no collection is
// given, or when the only argument isn't one, taking it as a request path.
func withSelectedCollection(opts runOptions) (runOptions, error) {
	if opts.collectionPath != "" && opts.requestPath != "" {
		return opts, nil
	}
	selected := postman.NewParser().LoadCLISelection().CollectionPath
	if opts.collectionPath == "" {
		if selected == "" {
			return opts, usageErrorFor(errNothingSelected, "pass a collection or select one with \"postOffice -c <collection>\"")
		}
		opts.collectionPath = selected
		return opts, nil
	}

	_, err := resolveCollectionRef(opts.collectionPath)
	if !errors.Is(err, errUnknownReference) || selected == "" || len(opts.folderPath) > 0 {
		return opts, nil
	}
	opts.collectionPath, opts.requestPath = selected, opts.collectionPath
	return opts, nil
}

// runTerminal attaches bash scripts to stdin when it is a terminal, so they
// can prompt. Their output goes to stderr, keeping stdout for the report.
// Without a terminal it is nil and scripts read no input.
func runTerminal() shell.Terminal {
	if !term.IsTerminal(os.Stdin.Fd()) {
		return nil
	}
	return shell.NewStdioTerminal(os.Stdin, os.Stderr, os.Stderr)
}
