package main

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"postOffice/internal/postman"
	"postOffice/internal/workflow"
)

// lsTopic is what `postOffice ls` lists.
type lsTopic struct {
	names []string
	list  func(out io.Writer, parsed parsedArgs) error
	flags []flagSpec
}

const (
	tabMinWidth = 0
	tabWidth    = 4
	tabPadding  = 2
	tabPadChar  = ' '

	noneListed = "  (none)"

	selectedPrefix   = "* "
	unselectedPrefix = "  "
	selectedLegend   = "\n* selected (see \"postOffice use\")"

	collectionNameSeparator = ", "
)

var lsTopics = []lsTopic{
	{names: []string{"var", "vars", "variables"}, list: listVariables, flags: []flagSpec{collectionFlag, environmentFlag}},
	{names: []string{"request", "requests", "r"}, list: listRequests, flags: []flagSpec{collectionFlag}},
}

var lsCommand = &cliCommand{
	name:      "ls",
	summary:   "List loaded collections, environments, workflows, variables or requests",
	aliasable: true,
	usage: `Usage:
  postOffice ls                                        collections, environments and workflows
  postOffice ls var [-c <collection>] [-e <env>]
  postOffice ls request [-c <collection>]

"ls" lists what the TUI has loaded (~/.postoffice_collections.json) and the
workflows saved next to each collection.
"ls var" (vars, variables) lists collection and environment variables with
their source; -c and -e list one collection or environment (a file or a
loaded name) instead of the loaded ones.
"ls request" (requests, r) lists requests with their method as
"Folder/Sub/Request" paths, exactly as "postOffice run" accepts them; -c lists
one collection file instead of the loaded ones.`,
	run: func(out io.Writer, args []string) error {
		if len(args) == 0 {
			return listOverview(out)
		}
		topicIndex := slices.IndexFunc(lsTopics, func(t lsTopic) bool { return slices.Contains(t.names, args[0]) })
		if topicIndex < 0 {
			return usageErrorf("unknown ls topic %q", args[0])
		}
		topic := lsTopics[topicIndex]
		parsed, err := parseFlags(args[1:], topic.flags...)
		if err != nil {
			return err
		}
		if len(parsed.positional) > 0 {
			return usageErrorf("unexpected argument %q", parsed.positional[0])
		}
		return topic.list(out, parsed)
	},
}

func newTable(out io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(out, tabMinWidth, tabWidth, tabPadding, tabPadChar, 0)
}

func loadedState() (*postman.Parser, error) {
	parser := postman.NewParser()
	if err := parser.LoadState(); err != nil {
		return nil, fmt.Errorf("failed to load loaded collections: %w", err)
	}
	return parser, nil
}

func listOverview(out io.Writer) error {
	parser, err := loadedState()
	if err != nil {
		return err
	}

	selection := parser.LoadCLISelection()
	table := newTable(out)
	fmt.Fprintln(table, "Collections:")
	writeNamedPaths(table, parser.ListCollections(), parser.GetCollectionPath, selection.CollectionPath)
	fmt.Fprintln(table, "Environments:")
	writeNamedPaths(table, parser.ListEnvironments(), parser.GetEnvironmentPath, selection.EnvironmentPath)

	fmt.Fprintln(table, "Workflows:")
	saved, err := workflowsSavedNextTo(loadedCollectionPaths(parser))
	if err != nil {
		return err
	}
	collectionNames := make(map[string]string)
	for _, name := range parser.ListCollections() {
		path, _ := parser.GetCollectionPath(name)
		collectionNames[path] = name
	}
	for _, file := range saved {
		owners := make([]string, 0, len(file.collectionPaths))
		for _, path := range file.collectionPaths {
			owners = append(owners, collectionNames[path])
		}
		fmt.Fprintf(table, "%s%s\t%s\t%s\n", selectionMarker(file.workflow.Path, selection.WorkflowPath), file.workflow.Workflow.ID, file.workflow.Workflow.Name, strings.Join(owners, collectionNameSeparator))
	}
	if len(saved) == 0 {
		fmt.Fprintln(table, noneListed)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out, selectedLegend)
	return nil
}

func writeNamedPaths(out io.Writer, names []string, pathOf func(string) (string, bool), selectedPath string) {
	if len(names) == 0 {
		fmt.Fprintln(out, noneListed)
		return
	}
	for _, name := range names {
		path, _ := pathOf(name)
		fmt.Fprintf(out, "%s%s\t%s\n", selectionMarker(path, selectedPath), name, path)
	}
}

// selectionMarker prefixes a listed file, marking the selected one.
func selectionMarker(path, selectedPath string) string {
	if path != "" && path == selectedPath {
		return selectedPrefix
	}
	return unselectedPrefix
}

// selectedCollections returns the collection given with -c, or every loaded one.
func selectedCollections(parser *postman.Parser, parsed parsedArgs) ([]*postman.Collection, error) {
	if parsed.has(collectionFlag) {
		path, err := resolveCollectionRef(parsed.value(collectionFlag))
		if err != nil {
			return nil, err
		}
		collection, err := parser.LoadCollection(path)
		if err != nil {
			return nil, err
		}
		return []*postman.Collection{collection}, nil
	}
	if err := parser.LoadState(); err != nil {
		return nil, fmt.Errorf("failed to load loaded collections: %w", err)
	}
	var collections []*postman.Collection
	for _, name := range parser.ListCollections() {
		collection, _ := parser.GetCollection(name)
		collections = append(collections, collection)
	}
	return collections, nil
}

// selectedEnvironments returns the environment given with -e, or every loaded one.
func selectedEnvironments(parser *postman.Parser, parsed parsedArgs) ([]*postman.Environment, error) {
	if parsed.has(environmentFlag) {
		path, err := resolveEnvironmentRef(parsed.value(environmentFlag))
		if err != nil {
			return nil, err
		}
		environment, err := parser.LoadEnvironment(path)
		if err != nil {
			return nil, err
		}
		return []*postman.Environment{environment}, nil
	}
	if err := parser.LoadState(); err != nil {
		return nil, fmt.Errorf("failed to load loaded environments: %w", err)
	}
	var environments []*postman.Environment
	for _, name := range parser.ListEnvironments() {
		environment, _ := parser.GetEnvironment(name)
		environments = append(environments, environment)
	}
	return environments, nil
}

func listVariables(out io.Writer, parsed parsedArgs) error {
	collections, err := selectedCollections(postman.NewParser(), parsed)
	if err != nil {
		return err
	}
	environments, err := selectedEnvironments(postman.NewParser(), parsed)
	if err != nil {
		return err
	}

	var variables []postman.VariableSource
	for _, collection := range collections {
		variables = append(variables, postman.GetAllVariables(collection, nil, nil)...)
	}
	for _, environment := range environments {
		variables = append(variables, postman.GetAllVariables(nil, nil, environment)...)
	}

	table := newTable(out)
	fmt.Fprintln(table, "KEY\tVALUE\tSOURCE")
	for _, variable := range variables {
		fmt.Fprintf(table, "%s\t%s\t%s\n", variable.Key, variable.Value, variable.Source)
	}
	return table.Flush()
}

func listRequests(out io.Writer, parsed parsedArgs) error {
	collections, err := selectedCollections(postman.NewParser(), parsed)
	if err != nil {
		return err
	}

	table := newTable(out)
	for i, collection := range collections {
		if i > 0 {
			fmt.Fprintln(table)
		}
		fmt.Fprintf(table, "%s:\n", collection.Info.Name)
		for _, ref := range workflow.RequestRefs(collection.Items, nil) {
			fmt.Fprintf(table, "  %s\t%s\n", ref.Item.Request.Method, ref.Path)
		}
	}
	return table.Flush()
}
