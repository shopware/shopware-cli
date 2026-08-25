package cmd

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

// runCLI executes run() with the given argv in an isolated environment: the
// update cache lands in a temp dir, tracking is disabled, and os.Args plus
// the mutated cobra state are restored afterwards. Keep argv[0] at
// "shopware-cli" so rootCmd.Use stays stable for the other tests, and do not
// use t.Parallel in tests calling this.
func runCLI(t *testing.T, argv ...string) (int, string) {
	t.Helper()

	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("DO_NOT_TRACK", "1")

	oldArgs := os.Args
	os.Args = argv
	out := new(bytes.Buffer)
	rootCmd.SetOut(out)
	rootCmd.SetErr(out)
	t.Cleanup(func() {
		os.Args = oldArgs
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		// cobra defines the version flag lazily on the first Execute, so it may
		// not exist yet; reset it so a persisted --version cannot short-circuit
		// a later run() call.
		if f := rootCmd.Flags().Lookup("version"); f != nil {
			_ = f.Value.Set("false")
			f.Changed = false
		}
	})

	return run(context.Background()), out.String()
}

func TestRunVersionFlagReturnsZero(t *testing.T) {
	code, out := runCLI(t, "shopware-cli", "--version")
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "dev")
}

func TestRunUnknownCommandReturnsOne(t *testing.T) {
	code, _ := runCLI(t, "shopware-cli", "definitely-not-a-command")
	assert.Equal(t, 1, code)
}

func TestCommandNameFromBinaryPathFallsBackForExtensionOnlyName(t *testing.T) {
	assert.Equal(t, "shopware-cli", commandNameFromBinaryPath(".hidden"))
	assert.Nil(t, mapAliasArgs(nil))
}
