package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

const (
	workflowRunSubcommand = "run"
	collectionFileExt     = ".json"
)

var (
	errCollectionNotFound  = errors.New("no collection found for the workflow")
	errAmbiguousCollection = errors.New("more than one collection could own the workflow")
	errNoWorkflowSelected  = errors.New("no workflow given and none selected")
)

// workflowRunOptions is a parsed `workflow run` command line.
type workflowRunOptions struct {
	workflowPath   string
	collectionPath string
	execution      executionOptions
}

func parseWorkflowRunArgs(args []string) (workflowRunOptions, error) {
	if len(args) == 0 || args[0] != workflowRunSubcommand {
		return workflowRunOptions{}, usageErrorf("expected \"workflow %s <workflow.yaml>\"", workflowRunSubcommand)
	}
	parsed, err := parseFlags(args[1:], append([]flagSpec{collectionFlag}, executionFlags...)...)
	if err != nil {
		return workflowRunOptions{}, err
	}
	if len(parsed.positional) > 1 {
		return workflowRunOptions{}, usageErrorf("workflow run takes one workflow, got %d", len(parsed.positional))
	}

	opts := workflowRunOptions{collectionPath: parsed.value(collectionFlag)}
	if len(parsed.positional) == 1 {
		opts.workflowPath = parsed.positional[0]
	}
	opts.execution, err = parseExecutionOptions(parsed)
	return opts, err
}

var workflowCommand = &cliCommand{
	name:      "workflow",
	aliases:   []string{"wf"},
	summary:   "Run a workflow YAML file",
	aliasable: true,
	usage: `Usage:
  postOffice workflow run [<workflow>] [-c <collection>] [options]

Runs a workflow against its collection. <workflow> is a workflow file, or the
ID, name or file name of a workflow saved next to a loaded collection (next to
-c's collection when given), ignoring case. Collections and environments can
be given by file or by loaded name, as in "postOffice run". Without
<workflow> the selected workflow runs, and without -e the selected
environment is used (see "postOffice use").

Without -c the collection of a workflow file is found from where the workflow
lives: workflows are saved in a "workflows" directory next to their collection
file, so postOffice uses
  1. the loaded collection (see "postOffice ls") whose workflows directory
     holds the file; when several loaded collections share that directory,
     the selected collection among them, or else
  2. the only Postman collection .json file in the directory above it.
Pass -c when neither finds exactly one collection.

Options:
  -c, --collection <collection>        collection the workflow runs against
  -e, --environment <env>              environment to resolve variables from
  --output text|json|junit             report format (default text)
  --report <file>                      write the json/junit report to a file
                                       and print the text report
  --persist                            save script variable changes back to the
                                       collection and environment files
  -n, --iterations <n>                 run n times (default: one per --data row, else 1)
  -d, --data <file.csv|json>           one iteration per row; values are available
                                       as {{key}} and pm.iterationData
  --bail                               stop at the first failed request, test or script

Exit codes: 0 all passed, 1 a request, test or script failed, 2 usage or input error,
130 interrupted (ctrl+c stops the run; variables are not persisted).`,
	run: func(out io.Writer, args []string) error {
		opts, err := parseWorkflowRunArgs(args)
		if err != nil {
			return err
		}
		if opts.workflowPath == "" {
			if opts.workflowPath = postman.NewParser().LoadCLISelection().WorkflowPath; opts.workflowPath == "" {
				return usageErrorFor(errNoWorkflowSelected, "pass a workflow or select one with \"postOffice -w <workflow>\"")
			}
		}
		collectionPath := opts.collectionPath
		if collectionPath != "" {
			if collectionPath, err = resolveCollectionRef(collectionPath); err != nil {
				return err
			}
		}
		workflowPath, err := resolveWorkflowRef(opts.workflowPath, collectionPath)
		if err != nil {
			return err
		}
		wf, err := workflow.LoadWorkflow(workflowPath)
		if err != nil {
			return err
		}
		if collectionPath == "" {
			if collectionPath, err = findWorkflowCollection(workflowPath); err != nil {
				return err
			}
		}
		return executeRun(out, collectionPath, opts.execution, func(*postman.Collection) (*workflow.Workflow, error) {
			return wf, nil
		})
	},
}

// findWorkflowCollection finds the collection a workflow file belongs to, as
// described in the workflow command's usage.
func findWorkflowCollection(workflowPath string) (string, error) {
	workflowsDir, err := filepath.Abs(filepath.Dir(workflowPath))
	if err != nil {
		return "", err
	}

	parser := postman.NewParser()
	if err := parser.LoadState(); err != nil {
		return "", fmt.Errorf("failed to load loaded collections: %w", err)
	}
	var loaded []string
	for _, path := range loadedCollectionPaths(parser) {
		if workflow.WorkflowsDir(path) == workflowsDir {
			loaded = append(loaded, path)
		}
	}
	if selected := parser.LoadCLISelection().CollectionPath; len(loaded) > 1 && slices.Contains(loaded, selected) {
		return selected, nil
	}
	if len(loaded) > 0 {
		return singleCollection(loaded, workflowPath)
	}

	siblings, err := postmanCollectionsIn(filepath.Dir(workflowsDir))
	if err != nil {
		return "", err
	}
	return singleCollection(siblings, workflowPath)
}

func singleCollection(candidates []string, workflowPath string) (string, error) {
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		return "", usageErrorFor(errCollectionNotFound, "%s; pass -c <collection.json>", workflowPath)
	default:
		return "", usageErrorFor(errAmbiguousCollection, "%s; pass -c <collection.json>", strings.Join(candidates, ", "))
	}
}

func postmanCollectionsIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", dir, err)
	}
	var collections []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != collectionFileExt {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}
		if postman.DetectFormat(data) == postman.FormatPostman {
			collections = append(collections, path)
		}
	}
	return collections, nil
}
