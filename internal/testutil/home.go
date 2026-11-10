// Package testutil holds helpers shared by test packages.
package testutil

import (
	"fmt"
	"os"
	"testing"
)

const isolatedHomePattern = "postoffice-test-home-*"

// RunWithIsolatedHome points HOME at a throwaway directory for the whole test
// binary, so no test can read or write the real ~/.postoffice_* config files
// (they may be symlinks into a dotfiles repo). Call it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testutil.RunWithIsolatedHome(m)) }
func RunWithIsolatedHome(m *testing.M) int {
	home, err := os.MkdirTemp("", isolatedHomePattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "testutil: failed to create isolated HOME: %v\n", err)
		return 1
	}
	defer os.RemoveAll(home)

	if err := os.Setenv("HOME", home); err != nil {
		fmt.Fprintf(os.Stderr, "testutil: failed to set HOME: %v\n", err)
		return 1
	}
	return m.Run()
}
