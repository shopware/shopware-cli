package verifier

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/logging"
)

func TestSetupToolsFailureLogging(t *testing.T) {
	for _, failingTool := range []string{"composer", "npm"} {
		t.Run(failingTool, func(t *testing.T) {
			for _, mode := range []struct {
				name     string
				disabled bool
			}{
				{name: "enabled"},
				{name: "disabled", disabled: true},
			} {
				t.Run(mode.name, func(t *testing.T) {
					t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
					t.Setenv("SHOPWARE_CLI_TOOLS_DIR", "")
					t.Setenv("PHP_BINARY", "")
					scripts := map[string]string{
						"php":      "printf 'PHP 8.2.0 (cli)'",
						"node":     "printf 'v20.0.0'",
						"composer": "exit 0",
						"npm":      "exit 0",
					}
					scripts[failingTool] = "printf 'Dependency install diagnostic'; exit 7"
					fakeTools(t, scripts)

					core, logs := observer.New(zap.InfoLevel)
					ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())
					if mode.disabled {
						ctx = logging.DisableLogger(ctx)
					}

					err := SetupTools(ctx, "logging-test")
					var exitError *exec.ExitError
					require.ErrorAs(t, err, &exitError)
					assert.Equal(t, 7, exitError.ExitCode())

					if mode.disabled {
						assert.Empty(t, logs.All())
					} else {
						entries := logs.FilterMessage("Dependency install diagnostic").All()
						require.Len(t, entries, 1)
						assert.Equal(t, zap.ErrorLevel, entries[0].Level)
					}
				})
			}
		})
	}
}
