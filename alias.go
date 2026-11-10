package main

import (
	"fmt"
	"io"
	"postOffice/internal/alias"
	"sort"
	"strings"
)

var aliasCommand = &cliCommand{
	name:    "alias",
	summary: "Save and replay headless commands under a short name",
	usage: `Usage:
  postOffice alias add <name> <command> [args...]   save a command, e.g. "run" or "merge"
  postOffice alias remove <name>
  postOffice alias list
  postOffice alias run <name>

Aliases are stored in ~/.postoffice_aliases.json. Any command marked as
aliasable (run, workflow, ls, merge) can be saved.`,
	run: runAlias,
}

// aliasableCommand returns the headless command an alias may store and replay.
func aliasableCommand(name string) (*cliCommand, bool) {
	cmd, ok := lookupCommand(name)
	return cmd, ok && cmd.aliasable
}

func runAlias(out io.Writer, args []string) error {
	if len(args) == 0 {
		return usageErrorf("missing alias subcommand")
	}

	switch args[0] {
	case "add":
		return aliasAdd(out, args[1:])
	case "remove":
		return aliasRemove(out, args[1:])
	case "list":
		return aliasList(out)
	case "run":
		return aliasRun(out, args[1:])
	default:
		return usageErrorf("unknown alias subcommand: %s", args[0])
	}
}

func aliasAdd(out io.Writer, args []string) error {
	if len(args) < 2 {
		return usageErrorf("alias add needs a name and a command")
	}
	name, command, commandArgs := args[0], args[1], args[2:]

	if _, ok := aliasableCommand(command); !ok {
		return usageErrorf("unknown headless command: %s", command)
	}

	cfg, err := alias.Load()
	if err != nil {
		return fmt.Errorf("failed to load alias config: %w", err)
	}

	cfg.Set(name, alias.Alias{Command: command, Args: commandArgs})

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save alias config: %w", err)
	}

	fmt.Fprintf(out, "Saved alias %q -> %s %s\n", name, command, strings.Join(commandArgs, " "))
	return nil
}

func aliasRemove(out io.Writer, args []string) error {
	if len(args) != 1 {
		return usageErrorf("alias remove needs exactly one name")
	}
	name := args[0]

	cfg, err := alias.Load()
	if err != nil {
		return fmt.Errorf("failed to load alias config: %w", err)
	}

	if !cfg.Remove(name) {
		return fmt.Errorf("alias not found: %s", name)
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save alias config: %w", err)
	}

	fmt.Fprintf(out, "Removed alias %q\n", name)
	return nil
}

func aliasList(out io.Writer) error {
	cfg, err := alias.Load()
	if err != nil {
		return fmt.Errorf("failed to load alias config: %w", err)
	}

	names := make([]string, 0, len(cfg.Aliases))
	for name := range cfg.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	if len(names) == 0 {
		fmt.Fprintln(out, "No aliases defined")
		return nil
	}

	for _, name := range names {
		a := cfg.Aliases[name]
		fmt.Fprintf(out, "%s: %s %s\n", name, a.Command, strings.Join(a.Args, " "))
	}
	return nil
}

func aliasRun(out io.Writer, args []string) error {
	if len(args) != 1 {
		return usageErrorf("alias run needs exactly one name")
	}
	name := args[0]

	cfg, err := alias.Load()
	if err != nil {
		return fmt.Errorf("failed to load alias config: %w", err)
	}

	a, exists := cfg.Get(name)
	if !exists {
		return fmt.Errorf("alias not found: %s", name)
	}

	cmd, ok := aliasableCommand(a.Command)
	if !ok {
		return fmt.Errorf("alias %q references unknown headless command: %s", name, a.Command)
	}

	return cmd.run(out, a.Args)
}
