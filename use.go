package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"postOffice/internal/postman"
)

var (
	workflowFlag = flagSpec{names: []string{"--workflow", "-w"}, takesValue: true}
	clearFlag    = flagSpec{names: []string{"--clear"}}
)

// selectionFlags pick what `use` selects; given first on the command line
// without a command they run `use`, e.g. postOffice -e "Big tusks".
var selectionFlags = []flagSpec{collectionFlag, environmentFlag, workflowFlag}

const notSelected = "(none)"

var errNothingSelected = errors.New("no collection given and none selected")

var useCommand = &cliCommand{
	name:    "use",
	summary: "Select the collection, environment and workflow commands default to",
	usage: `Usage:
  postOffice use [-c <collection>] [-e <environment>] [-w <workflow>]
  postOffice use --clear
  postOffice -c <collection> [-e <environment>] [-w <workflow>]

Selects what headless commands use when none is given: "run" runs the
selected collection and "workflow run" the selected workflow, both with the
selected environment unless -e is passed. Each is a file or a loaded name, as
in "postOffice run". An empty value ("") unselects one, --clear all of them.
Without options it shows the selection, which "postOffice ls" also marks with *.
The selection is stored in ~/.postoffice_session.json.`,
	run: runUse,
}

// isSelectionInvocation reports whether args start with a selection flag,
// which is shorthand for `use`.
func isSelectionInvocation(args []string) bool {
	return len(args) > 0 && slices.ContainsFunc(selectionFlags, func(flag flagSpec) bool {
		return slices.Contains(flag.names, flagName(args[0]))
	})
}

func flagName(arg string) string {
	name, _, _ := strings.Cut(arg, "=")
	return name
}

func runUse(out io.Writer, args []string) error {
	parsed, err := parseFlags(args, append(slices.Clone(selectionFlags), clearFlag)...)
	if err != nil {
		return err
	}
	if len(parsed.positional) > 0 {
		return usageErrorf("unexpected argument %q", parsed.positional[0])
	}

	parser := postman.NewParser()
	selection := parser.LoadCLISelection()
	if parsed.has(clearFlag) {
		selection = &postman.CLISelection{}
	}
	if err := applySelection(selection, parsed); err != nil {
		return err
	}
	if parsed.has(clearFlag) || slices.ContainsFunc(selectionFlags, parsed.has) {
		if err := parser.SaveCLISelection(selection); err != nil {
			return err
		}
	}
	writeSelection(out, selection)
	return nil
}

// applySelection resolves each given selection flag into selection; the
// workflow is looked up next to the newly or already selected collection.
func applySelection(selection *postman.CLISelection, parsed parsedArgs) error {
	if parsed.has(collectionFlag) {
		path, err := resolveOptionalRef(parsed.value(collectionFlag), resolveCollectionRef)
		if err != nil {
			return err
		}
		selection.CollectionPath = path
	}
	if parsed.has(environmentFlag) {
		path, err := resolveOptionalRef(parsed.value(environmentFlag), resolveEnvironmentRef)
		if err != nil {
			return err
		}
		selection.EnvironmentPath = path
	}
	if parsed.has(workflowFlag) {
		path, err := resolveOptionalRef(parsed.value(workflowFlag), func(ref string) (string, error) {
			return resolveWorkflowRef(ref, selection.CollectionPath)
		})
		if err != nil {
			return err
		}
		selection.WorkflowPath = path
	}
	return nil
}

// resolveOptionalRef resolves ref, leaving an empty ref empty so it unselects.
func resolveOptionalRef(ref string, resolve func(string) (string, error)) (string, error) {
	if ref == "" {
		return "", nil
	}
	return resolve(ref)
}

func writeSelection(out io.Writer, selection *postman.CLISelection) {
	table := newTable(out)
	fmt.Fprintln(table, "Selected:")
	fmt.Fprintf(table, "  collection\t%s\n", orNotSelected(selection.CollectionPath))
	fmt.Fprintf(table, "  environment\t%s\n", orNotSelected(selection.EnvironmentPath))
	fmt.Fprintf(table, "  workflow\t%s\n", orNotSelected(selection.WorkflowPath))
	_ = table.Flush()
}

func orNotSelected(path string) string {
	if path == "" {
		return notSelected
	}
	return path
}

// withSelectedEnvironment fills in the selected environment when -e wasn't given.
func withSelectedEnvironment(opts executionOptions) executionOptions {
	if opts.environmentPath == "" {
		opts.environmentPath = postman.NewParser().LoadCLISelection().EnvironmentPath
	}
	return opts
}
