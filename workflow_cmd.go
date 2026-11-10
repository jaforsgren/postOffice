package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	if len(parsed.positional) != 1 {
		return workflowRunOptions{}, usageErrorf("workflow run takes exactly one workflow file, got %d", len(parsed.positional))
	}

	opts := workflowRunOptions{workflowPath: parsed.positional[0], collectionPath: parsed.value(collectionFlag)}
	opts.execution, err = parseExecutionOptions(parsed)
	return opts, err
}

var workflowCommand = &cliCommand{
	name:      "workflow",
	aliases:   []string{"wf"},
	summary:   "Run a workflow YAML file",
	aliasable: true,
	usage: `Usage:
  postOffice workflow run <workflow.yaml> [-c <collection.json>] [options]

Runs a workflow file against its collection. Without -c the collection is
found from where the workflow lives: workflows are saved in a "workflows"
directory next to their collection file, so postOffice uses
  1. the loaded collection (see "postOffice ls") whose workflows directory
     holds the file, or else
  2. the only Postman collection .json file in the directory above it.
Pass -c when neither finds exactly one collection.

Options:
  -c, --collection <collection.json>   collection the workflow runs against
  -e, --environment <env.json>         environment to resolve variables from
  --output text|json|junit             report format (default text)
  --report <file>                      write the json/junit report to a file
                                       and print the text report
  --persist                            save script variable changes back to the
                                       collection and environment files

Exit codes: 0 all passed, 1 a request, test or script failed, 2 usage or input error.`,
	run: func(out io.Writer, args []string) error {
		opts, err := parseWorkflowRunArgs(args)
		if err != nil {
			return err
		}
		wf, err := workflow.LoadWorkflow(opts.workflowPath)
		if err != nil {
			return err
		}
		collectionPath := opts.collectionPath
		if collectionPath == "" {
			if collectionPath, err = findWorkflowCollection(opts.workflowPath); err != nil {
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
	for _, name := range parser.ListCollections() {
		if path, ok := parser.GetCollectionPath(name); ok && workflow.WorkflowsDir(path) == workflowsDir {
			loaded = append(loaded, path)
		}
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
