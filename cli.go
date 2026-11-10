package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

const programName = "postOffice"

// Exit codes for headless commands.
const (
	// exitSuccess: the command completed and every request, test and script passed.
	exitSuccess = 0
	// exitRunFailed: a run completed but a request, test or script failed.
	exitRunFailed = 1
	// exitUsage: bad arguments, or input that could not be read or parsed
	// (missing file, invalid JSON/YAML, unknown request or folder).
	exitUsage = 2
)

var (
	// errUsage marks errors in how a command was invoked; the command's usage is suggested.
	errUsage = errors.New("usage error")
	// errRunFailed marks a run that completed with failures; its report already explains them.
	errRunFailed = errors.New("run failed")
)

var helpFlags = []string{"-h", "--help"}

// cliCommand is one headless subcommand.
type cliCommand struct {
	name    string
	aliases []string
	summary string
	usage   string
	run     func(out io.Writer, args []string) error
	// aliasable commands can be saved and replayed with `postOffice alias`.
	aliasable bool
}

// cliCommands lists the headless subcommands in help order. It is filled in
// init because the help and alias commands refer back to it.
var cliCommands []*cliCommand

func init() {
	cliCommands = []*cliCommand{runCommand, workflowCommand, lsCommand, useCommand, mergeCommand, aliasCommand, helpCommand}
}

func lookupCommand(name string) (*cliCommand, bool) {
	for _, cmd := range cliCommands {
		if cmd.name == name || slices.Contains(cmd.aliases, name) {
			return cmd, true
		}
	}
	return nil, false
}

// isTUIInvocation reports whether args start the interactive TUI: no
// arguments, or only the TUI's own flags such as --log.
func isTUIInvocation(args []string) bool {
	return len(args) == 0 || strings.HasPrefix(args[0], "-") && !slices.Contains(helpFlags, args[0]) && !isCommandName(args[0]) && !isSelectionInvocation(args)
}

func isCommandName(name string) bool {
	_, ok := lookupCommand(name)
	return ok
}

func wantsHelp(args []string) bool {
	return slices.ContainsFunc(args, func(arg string) bool { return slices.Contains(helpFlags, arg) })
}

// runCLI runs a headless command and returns the process exit code.
func runCLI(args []string, stdout, stderr io.Writer) int {
	if slices.Contains(helpFlags, args[0]) {
		writeUsage(stdout)
		return exitSuccess
	}
	if isSelectionInvocation(args) {
		args = append([]string{useCommand.name}, args...)
	}

	cmd, ok := lookupCommand(args[0])
	if !ok {
		fmt.Fprintf(stderr, "Error: unknown command %q\n\n", args[0])
		writeUsage(stderr)
		return exitUsage
	}

	if cmd != helpCommand && wantsHelp(args[1:]) {
		fmt.Fprintln(stdout, cmd.usage)
		return exitSuccess
	}

	err := cmd.run(stdout, args[1:])
	switch {
	case err == nil:
		return exitSuccess
	case errors.Is(err, errRunFailed):
		return exitRunFailed
	case errors.Is(err, errUsage):
		fmt.Fprintf(stderr, "Error: %v\n\n%s\n", err, cmd.usage)
		return exitUsage
	default:
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return exitUsage
	}
}

// usageError is an invocation mistake. It matches errUsage with errors.Is and
// unwraps to its optional cause, so callers can also test for that.
type usageError struct {
	message string
	cause   error
}

func (e *usageError) Error() string { return e.message }

func (e *usageError) Is(target error) bool { return target == errUsage }

func (e *usageError) Unwrap() error { return e.cause }

func usageErrorf(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

// usageErrorFor is a usage error caused by err, such as a missing -c that the
// command could not work out on its own.
func usageErrorFor(err error, format string, args ...any) error {
	return &usageError{message: fmt.Sprintf("%v: %s", err, fmt.Sprintf(format, args...)), cause: err}
}

var helpCommand = &cliCommand{
	name:    "help",
	summary: "Show help for postOffice or one of its commands",
	usage: `Usage: postOffice help [command]

Shows the overall usage, or the usage of one command.
"postOffice <command> --help" and "postOffice --help" do the same.`,
	run: func(out io.Writer, args []string) error {
		if len(args) == 0 {
			writeUsage(out)
			return nil
		}
		cmd, ok := lookupCommand(args[0])
		if !ok {
			return usageErrorf("unknown command %q", args[0])
		}
		fmt.Fprintln(out, cmd.usage)
		return nil
	},
}

func writeUsage(out io.Writer) {
	fmt.Fprintf(out, "Usage:\n  %s [--log <file>]          start the interactive TUI\n  %s <command> [arguments]   run a headless command\n  %s -c|-e|-w <name>         select what commands default to (see \"use\")\n\nCommands:\n", programName, programName, programName)
	for _, cmd := range cliCommands {
		name := cmd.name
		if len(cmd.aliases) > 0 {
			name += " (" + strings.Join(cmd.aliases, ", ") + ")"
		}
		fmt.Fprintf(out, "  %-24s %s\n", name, cmd.summary)
	}
	fmt.Fprintf(out, `
Exit codes:
  %d  success: every request, test and script passed
  %d  a run finished with a failed request, test or script
  %d  usage or input error (bad arguments, unreadable file, invalid JSON/YAML, unknown request)

Run "%s <command> --help" for a command's options.
`, exitSuccess, exitRunFailed, exitUsage, programName)
}

// flagSpec describes one flag a command accepts. The first name is the key
// its value is stored under.
type flagSpec struct {
	names      []string
	takesValue bool
}

func (f flagSpec) key() string { return f.names[0] }

var (
	collectionFlag  = flagSpec{names: []string{"--collection", "-c"}, takesValue: true}
	environmentFlag = flagSpec{names: []string{"--environment", "-e"}, takesValue: true}
)

// parsedArgs is a command line split into positional arguments and flags.
type parsedArgs struct {
	positional []string
	values     map[string]string
	present    map[string]bool
}

func (p parsedArgs) value(flag flagSpec) string { return p.values[flag.key()] }

func (p parsedArgs) has(flag flagSpec) bool { return p.present[flag.key()] }

// parseFlags accepts "--flag value", "--flag=value" and boolean flags, in any
// order around the positional arguments.
func parseFlags(args []string, specs ...flagSpec) (parsedArgs, error) {
	parsed := parsedArgs{values: map[string]string{}, present: map[string]bool{}}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			parsed.positional = append(parsed.positional, arg)
			continue
		}

		name, inlineValue, hasInlineValue := strings.Cut(arg, "=")
		specIndex := slices.IndexFunc(specs, func(f flagSpec) bool { return slices.Contains(f.names, name) })
		if specIndex < 0 {
			return parsedArgs{}, usageErrorf("unknown flag %s", name)
		}
		spec := specs[specIndex]
		parsed.present[spec.key()] = true

		switch {
		case !spec.takesValue && hasInlineValue:
			return parsedArgs{}, usageErrorf("%s does not take a value", name)
		case !spec.takesValue:
		case hasInlineValue:
			parsed.values[spec.key()] = inlineValue
		case i+1 < len(args):
			i++
			parsed.values[spec.key()] = args[i]
		default:
			return parsedArgs{}, usageErrorf("%s requires a value", name)
		}
	}
	return parsed, nil
}
