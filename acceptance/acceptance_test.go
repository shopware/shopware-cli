package acceptance

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"shopware-cli": runCLI,
		"swx":          runCLI,
	})
}

// TestScripts runs every testdata/*.txtar against the real CLI.
func TestScripts(t *testing.T) {
	fakes := httptest.NewServer(fakeServices())
	t.Cleanup(fakes.Close)

	testscript.Run(t, testscript.Params{
		Dir:                 "testdata",
		RequireExplicitExec: true,
		Setup: func(env *testscript.Env) error {
			// Keep config and cache files inside the script's work directory.
			env.Setenv("HOME", env.WorkDir)
			// Scripts run offline: no telemetry, no update check, Packagist and static data from the fakes.
			env.Setenv("DO_NOT_TRACK", "1")
			env.Setenv("SHOPWARE_CLI_NO_UPDATE_NOTIFICATION", "true")
			env.Setenv("ACCEPTANCE_FAKE_URL", fakes.URL)
			// A request the CLI could not make offline fails the script when it ends.
			refused := filepath.Join(env.WorkDir, ".tmp", "refused-requests")
			env.Setenv("ACCEPTANCE_REFUSED_LOG", refused)
			env.Defer(func() {
				if requests, err := os.ReadFile(refused); err == nil && len(requests) > 0 {
					env.T().Fatal("script reached the network, refused:\n" + string(requests))
				}
			})

			return nil
		},
	})
}
