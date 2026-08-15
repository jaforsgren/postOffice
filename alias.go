package main

import (
	"fmt"
	"postOffice/internal/alias"
	"sort"
	"strings"
)

// headlessCommands maps a headless command name to its handler. An alias
// stores one of these names plus the args to replay. Register future
// headless commands (e.g. "validate", "convert") here to make them
// available both directly and via `postOffice alias`.
var headlessCommands = map[string]func([]string) error{
	"merge": runMerge,
}

func runAlias(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: postOffice alias <add|remove|list|run> ...")
	}

	switch args[0] {
	case "add":
		return aliasAdd(args[1:])
	case "remove":
		return aliasRemove(args[1:])
	case "list":
		return aliasList()
	case "run":
		return aliasRun(args[1:])
	default:
		return fmt.Errorf("unknown alias subcommand: %s", args[0])
	}
}

func aliasAdd(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: postOffice alias add <name> <command> [args...]")
	}
	name, command, commandArgs := args[0], args[1], args[2:]

	if _, ok := headlessCommands[command]; !ok {
		return fmt.Errorf("unknown headless command: %s", command)
	}

	cfg, err := alias.Load()
	if err != nil {
		return fmt.Errorf("failed to load alias config: %w", err)
	}

	cfg.Set(name, alias.Alias{Command: command, Args: commandArgs})

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save alias config: %w", err)
	}

	fmt.Printf("Saved alias %q -> %s %s\n", name, command, strings.Join(commandArgs, " "))
	return nil
}

func aliasRemove(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: postOffice alias remove <name>")
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

	fmt.Printf("Removed alias %q\n", name)
	return nil
}

func aliasList() error {
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
		fmt.Println("No aliases defined")
		return nil
	}

	for _, name := range names {
		a := cfg.Aliases[name]
		fmt.Printf("%s: %s %s\n", name, a.Command, strings.Join(a.Args, " "))
	}
	return nil
}

func aliasRun(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: postOffice alias run <name>")
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

	handler, ok := headlessCommands[a.Command]
	if !ok {
		return fmt.Errorf("alias %q references unknown headless command: %s", name, a.Command)
	}

	return handler(a.Args)
}
