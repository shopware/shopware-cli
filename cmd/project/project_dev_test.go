package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestSetupDevEnvironmentRejectsOldCompatibilityDate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("compatibility_date: 2020-01-01\n"), 0o644))
	t.Setenv("PROJECT_ROOT", dir)

	// Register never runs in the test binary, so the global defaults to "" and
	// ReadConfig would silently fall back to a modern compatibility date.
	oldPath := projectConfigPath
	projectConfigPath = configPath
	t.Cleanup(func() { projectConfigPath = oldPath })

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	_, err := setupDevEnvironment(cmd)
	require.ErrorIs(t, err, shop.ErrDevModeNotSupported)
}
