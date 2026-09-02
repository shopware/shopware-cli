package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	internalextension "github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/system"
)

func TestCreateCommandDefaults(t *testing.T) {
	cmd := newCreateCmd()

	usage, err := cmd.Flags().GetString("usage")
	require.NoError(t, err)
	assert.Equal(t, string(internalextension.PrivateUsage), usage)
}

func TestCreateCommandMapsFlags(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "plugins"), 0o755))

	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{
		"--usage", "store",
		"SwagBasicExample",
	})

	require.NoError(t, cmd.Execute())
	assert.FileExists(t, filepath.Join(
		projectDir,
		"custom",
		"plugins",
		"SwagBasicExample",
		"composer.json",
	))
}

func TestCreateCommandValidatesUsageFlag(t *testing.T) {
	tests := []struct {
		name      string
		usage     string
		pluginDir string
		error     string
	}{
		{
			name:      "private",
			usage:     "private",
			pluginDir: "static-plugins",
		},
		{
			name:      "store",
			usage:     "store",
			pluginDir: "plugins",
		},
		{
			name:  "rejected",
			usage: "unknown",
			error: `invalid extension usage "unknown"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			t.Setenv("PROJECT_ROOT", projectDir)
			require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "plugins"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))

			cmd := newCreateCmd()
			cmd.SetContext(system.WithInteraction(t.Context(), false))
			cmd.SetArgs([]string{"--usage", test.usage, "SwagBasicExample"})

			err := cmd.Execute()
			if test.error != "" {
				assert.ErrorContains(t, err, test.error)
				assert.NoDirExists(t, filepath.Join(projectDir, "custom", "plugins", "SwagBasicExample"))
				assert.NoDirExists(t, filepath.Join(projectDir, "custom", "static-plugins", "SwagBasicExample"))
				return
			}

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(
				projectDir,
				"custom",
				test.pluginDir,
				"SwagBasicExample",
				"composer.json",
			))
		})
	}
}

func TestCreateCommandAcceptsNameArgument(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))

	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{"SwagBasicExample"})

	require.NoError(t, cmd.Execute())
	assert.FileExists(t, filepath.Join(
		projectDir,
		"custom",
		"static-plugins",
		"SwagBasicExample",
		"composer.json",
	))
}

func TestCreateCommandRejectsSecondArgument(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{"SwagBasicExample", "Unexpected"})

	err := cmd.Execute()

	assert.ErrorContains(t, err, "accepts at most 1 arg(s), received 2")
}
