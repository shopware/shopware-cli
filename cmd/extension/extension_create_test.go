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

	extensionType, err := cmd.Flags().GetString("type")
	require.NoError(t, err)
	assert.Equal(t, string(internalextension.Plugin), extensionType)

	store, err := cmd.Flags().GetBool("store")
	require.NoError(t, err)
	assert.False(t, store)
}

func TestCreateCommandMapsFlags(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "plugins"), 0o755))

	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{
		"--store",
		"--name", "SwagBasicExample",
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

func TestCreateCommandValidatesTypeFlag(t *testing.T) {
	tests := []struct {
		name          string
		extensionType string
		error         string
	}{
		{
			name:          "plugin",
			extensionType: "plugin",
		},
		{
			name:          "theme",
			extensionType: "theme",
		},
		{
			name:          "rejected",
			extensionType: "unknown",
			error:         `invalid extension type "unknown"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectDir := t.TempDir()
			t.Setenv("PROJECT_ROOT", projectDir)
			require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))

			cmd := newCreateCmd()
			cmd.SetContext(system.WithInteraction(t.Context(), false))
			cmd.SetArgs([]string{"--type", test.extensionType, "--name", "SwagBasicExample"})

			err := cmd.Execute()
			if test.error != "" {
				assert.ErrorContains(t, err, test.error)
				assert.NoDirExists(t, filepath.Join(projectDir, "custom", "static-plugins", "SwagBasicExample"))
				return
			}

			require.NoError(t, err)
			assert.FileExists(t, filepath.Join(
				projectDir,
				"custom",
				"static-plugins",
				"SwagBasicExample",
				"composer.json",
			))
		})
	}
}

func TestCreateCommandValidatesNameFlag(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{"--name", "invalid-name"})

	err := cmd.Execute()

	assert.ErrorContains(t, err, "invalid extension name")
}

func TestCreateCommandAcceptsNameFlag(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))

	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{"--name", "SwagBasicExample"})

	require.NoError(t, cmd.Execute())
	assert.FileExists(t, filepath.Join(
		projectDir,
		"custom",
		"static-plugins",
		"SwagBasicExample",
		"composer.json",
	))
}

func TestCreateCommandRejectsArguments(t *testing.T) {
	cmd := newCreateCmd()
	cmd.SetContext(system.WithInteraction(t.Context(), false))
	cmd.SetArgs([]string{"SwagBasicExample"})

	err := cmd.Execute()

	assert.ErrorContains(t, err, "unknown command")
}
