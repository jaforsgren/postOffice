package shell

import (
	"context"
	"os/exec"
)

// Terminal runs a command attached to the user's terminal, so a script can
// prompt (e.g. `read -p "Continue? [y/n] "`) and the user sees its output.
// A Terminal adds itself to the command's input and output; the output
// writers already set on cmd keep receiving everything the script prints.
type Terminal interface {
	Run(ctx context.Context, cmd *exec.Cmd) error
}
