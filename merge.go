package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"strings"
)

var mergeCommand = &cliCommand{
	name:      "merge",
	aliases:   []string{"-m", "--merge"},
	summary:   "Merge two collections or two environments",
	aliasable: true,
	usage: `Usage:
  postOffice merge <file1.json> <file2.json> (--forceMerge | --keep-original) [options]
  postOffice -m <file1.json> <file2.json> ...

Merges two collections or two environments. Folders with the same name merge
recursively; same-named requests or variables follow the chosen strategy.

Options:
  --forceMerge                  file2 wins conflicts
  --keep-original               file1 wins conflicts
  -o, --output <file>           write the result here instead of over file1
  -g, --group-by-collection     nest each collection's items under a folder
                                named after it (collections only)`,
	run: runMerge,
}

type mergeOptions struct {
	file1             string
	file2             string
	output            string
	strategy          postman.MergeStrategy
	groupByCollection bool
}

// parseMergeArgs parses the arguments following "-m"/"--merge", e.g.
// "a.json b.json --forceMerge" or "a.json b.json --keep-original -o out.json".
func parseMergeArgs(args []string) (*mergeOptions, error) {
	var positional []string
	var output string
	forceMerge := false
	keepOriginal := false
	groupByCollection := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--forceMerge":
			forceMerge = true
		case "--keep-original":
			keepOriginal = true
		case "-g", "--group-by-collection":
			groupByCollection = true
		case "-o", "--output":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("%s requires a value", arg)
			}
			output = args[i]
		default:
			if strings.HasPrefix(arg, "-") {
				return nil, fmt.Errorf("unknown flag: %s", arg)
			}
			positional = append(positional, arg)
		}
	}

	if len(positional) != 2 {
		return nil, fmt.Errorf("merge requires exactly two input files, got %d", len(positional))
	}

	if forceMerge == keepOriginal {
		return nil, fmt.Errorf("exactly one of --forceMerge or --keep-original must be specified")
	}

	strategy := postman.MergeKeepOriginal
	if forceMerge {
		strategy = postman.MergeForce
	}

	return &mergeOptions{
		file1:             positional[0],
		file2:             positional[1],
		output:            output,
		strategy:          strategy,
		groupByCollection: groupByCollection,
	}, nil
}

func runMerge(out io.Writer, args []string) error {
	opts, err := parseMergeArgs(args)
	if err != nil {
		return err
	}

	data1, err := os.ReadFile(opts.file1)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", opts.file1, err)
	}

	data2, err := os.ReadFile(opts.file2)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", opts.file2, err)
	}

	isEnv1 := postman.IsEnvironmentFormat(data1)
	isEnv2 := postman.IsEnvironmentFormat(data2)
	if isEnv1 != isEnv2 {
		return fmt.Errorf("cannot merge a collection with an environment")
	}

	if opts.groupByCollection && isEnv1 {
		return fmt.Errorf("--group-by-collection only applies to collections, not environments")
	}

	output := opts.output
	if output == "" {
		output = opts.file1
	}

	var merged any
	if isEnv1 {
		merged, err = mergeEnvironmentFiles(data1, data2, opts.strategy)
	} else if opts.groupByCollection {
		merged, err = mergeCollectionFilesGrouped(data1, data2, opts.file1, opts.file2, opts.strategy)
	} else {
		merged, err = mergeCollectionFiles(data1, data2, opts.strategy)
	}
	if err != nil {
		return err
	}

	bytes, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal merged result: %w", err)
	}

	if err := os.WriteFile(output, bytes, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", output, err)
	}

	fmt.Fprintf(out, "Merged %s and %s -> %s\n", opts.file1, opts.file2, output)
	return nil
}

func mergeEnvironmentFiles(data1, data2 []byte, strategy postman.MergeStrategy) (*postman.Environment, error) {
	var env1, env2 postman.Environment
	if err := json.Unmarshal(data1, &env1); err != nil {
		return nil, fmt.Errorf("failed to parse environment: %w", err)
	}
	if err := json.Unmarshal(data2, &env2); err != nil {
		return nil, fmt.Errorf("failed to parse environment: %w", err)
	}
	return postman.MergeEnvironments(&env1, &env2, strategy), nil
}

func mergeCollectionFiles(data1, data2 []byte, strategy postman.MergeStrategy) (*postman.Collection, error) {
	col1, err := postman.ParseCollectionData(data1)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}
	col2, err := postman.ParseCollectionData(data2)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}
	return postman.MergeCollections(col1, col2, strategy), nil
}

func mergeCollectionFilesGrouped(data1, data2 []byte, path1, path2 string, strategy postman.MergeStrategy) (*postman.Collection, error) {
	col1, err := postman.ParseCollectionData(data1)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}
	col2, err := postman.ParseCollectionData(data2)
	if err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}

	group1 := collectionGroupName(col1, path1)
	group2 := collectionGroupName(col2, path2)

	return postman.MergeCollectionsGrouped(col1, col2, group1, group2, strategy), nil
}

// collectionGroupName names the top-level folder a collection's items get
// nested under in a grouped merge: the collection's own name, or its
// filename (without extension) if the collection has no name.
func collectionGroupName(col *postman.Collection, path string) string {
	if col.Info.Name != "" {
		return col.Info.Name
	}
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
