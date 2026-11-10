package tui

import (
	"os"
	"testing"

	"postOffice/internal/testutil"
)

func TestMain(m *testing.M) { os.Exit(testutil.RunWithIsolatedHome(m)) }
