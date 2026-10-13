package acceptance

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
	"github.com/shyim/go-composer/repository"

	"github.com/shopware/shopware-cli/cmd"
)

// runCLI is what the scripts execute; it matches main.
func runCLI() {
	os.Exit(cmd.Execute(context.Background()))
}

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"shopware-cli": runCLI,
		"swx":          runCLI,
	})
}

// TestScripts runs every testdata/*.txtar against the real CLI.
func TestScripts(t *testing.T) {
	packagist := httptest.NewServer(repository.NewHandler(shopwareCoreVersions{}))
	t.Cleanup(packagist.Close)

	testscript.Run(t, testscript.Params{
		Dir:                 "testdata",
		RequireExplicitExec: true,
		Setup: func(env *testscript.Env) error {
			// Keep config and cache files inside the script's work directory.
			env.Setenv("HOME", env.WorkDir)
			// Scripts run offline: no telemetry, no update check, a local Packagist.
			env.Setenv("DO_NOT_TRACK", "1")
			env.Setenv("SHOPWARE_CLI_NO_UPDATE_NOTIFICATION", "true")
			env.Setenv("SHOPWARE_CLI_PACKAGIST_URL", packagist.URL)
			return nil
		},
	})
}
