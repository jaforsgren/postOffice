package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

const reportFilePermissions = 0644

var (
	folderFlag  = flagSpec{names: []string{"--folder"}, takesValue: true}
	outputFlag  = flagSpec{names: []string{"--output"}, takesValue: true}
	reportFlag  = flagSpec{names: []string{"--report"}, takesValue: true}
	persistFlag = flagSpec{names: []string{"--persist"}}
)

// executionFlags are accepted by every command that sends requests. Runner
// options (such as --iterations, --bail and --data) are added here and to
// executionOptions, and so apply to `run` and `workflow run` alike.
var executionFlags = []flagSpec{environmentFlag, outputFlag, reportFlag, persistFlag}

// errPersistUnsupported is returned when --persist would overwrite a file that is not a Postman collection.
var errPersistUnsupported = errors.New("--persist only works with Postman collection files")

// executionOptions controls how a headless run executes and reports.
type executionOptions struct {
	environmentPath string
	output          outputFormat
	reportPath      string
	persist         bool
}

func parseExecutionOptions(parsed parsedArgs) (executionOptions, error) {
	opts := executionOptions{
		environmentPath: parsed.value(environmentFlag),
		output:          outputText,
		reportPath:      parsed.value(reportFlag),
		persist:         parsed.has(persistFlag),
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
	case 1:
		opts.collectionPath = parsed.positional[0]
	case 2:
		opts.collectionPath, opts.requestPath = parsed.positional[0], parsed.positional[1]
	default:
		return runOptions{}, usageErrorf("run takes a collection file and optionally one request path, got %d arguments", len(parsed.positional))
	}

	if parsed.has(folderFlag) {
		if opts.requestPath != "" {
			return runOptions{}, usageErrorf("give either a request path or --folder, not both")
		}
		folder := parsed.value(folderFlag)
		if folder == "" {
			return runOptions{}, usageErrorf("--folder needs a folder path such as \"Users/Admin\"")
		}
		opts.folderPath = strings.Split(folder, workflow.RequestPathSeparator)
	}

	opts.execution, err = parseExecutionOptions(parsed)
	return opts, err
}

var runCommand = &cliCommand{
	name:      "run",
	summary:   "Run a request, a folder or a whole collection with its scripts",
	aliasable: true,
	usage: `Usage:
  postOffice run <collection.json> "<Folder/Sub/Request>" [options]
  postOffice run <collection.json> --folder "<Folder/Sub>" [options]
  postOffice run <collection.json> [options]

Runs one request, every request in a folder, or every request in the
collection, with collection → folder → request pre-request and test scripts.
Folder and collection runs go in collection order and follow
pm.execution.setNextRequest and skipRequest, like Postman's Collection Runner.
Request paths are those listed by "postOffice ls request".

Options:
  -e, --environment <env.json>   environment to resolve variables from
  --folder <Folder/Sub>          run every request in this folder
  --output text|json|junit       report format (default text)
  --report <file>                write the json/junit report to a file and
                                 print the text report
  --persist                      save script variable changes back to the
                                 collection and environment files (off by default)

Exit codes: 0 all passed, 1 a request, test or script failed, 2 usage or input error.`,
	run: func(out io.Writer, args []string) error {
		opts, err := parseRunArgs(args)
		if err != nil {
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

// executeRun loads the collection and environment, runs the workflow
// buildWorkflow makes from the collection, and reports the result.
func executeRun(out io.Writer, collectionPath string, opts executionOptions, buildWorkflow func(*postman.Collection) (*workflow.Workflow, error)) error {
	if opts.persist {
		if err := requirePostmanCollection(collectionPath); err != nil {
			return err
		}
	}

	parser := postman.NewParser()
	collection, err := parser.LoadCollection(collectionPath)
	if err != nil {
		return err
	}
	var environment *postman.Environment
	if opts.environmentPath != "" {
		if environment, err = parser.LoadEnvironment(opts.environmentPath); err != nil {
			return err
		}
	}

	wf, err := buildWorkflow(collection)
	if err != nil {
		return err
	}

	state := workflow.NewRunner(collection, environment, internalhttp.NewExecutor()).Run(wf, nil)
	report := newRunReport(state, collection.Info.Name, environmentName(environment))
	if err := writeRunOutput(out, report, opts); err != nil {
		return err
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

// writeRunOutput prints the report in the chosen format. With --report the
// machine-readable report goes to that file and the text report to out.
func writeRunOutput(out io.Writer, report runReport, opts executionOptions) error {
	writeMachine, isMachine := machineWriters[opts.output]
	if !isMachine {
		writeTextReport(out, report)
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
	writeTextReport(out, report)
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
