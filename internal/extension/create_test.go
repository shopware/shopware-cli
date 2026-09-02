package extension

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
)

func TestValidateExtensionName(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"SwagBasicExample", "MyPlugin", "AcmePayPal"} {
		assert.NoError(t, validateExtensionNameInput(valid), valid)
	}

	for _, invalid := range []string{
		"",
		"swagBasicExample",
		"Swag",
		"my-plugin",
		"My_Plugin",
		"My Plugin",
		"1Plugin",
		"Swag.Example",
	} {
		assert.Error(t, validateExtensionNameInput(invalid), invalid)
	}
}

func TestValidatePluginInstallable(t *testing.T) {
	extensionDir := scaffoldPlugin(t, "SwagBasicExample")

	check := &testCheck{}
	validatePluginInstallable(loadPlugin(t, extensionDir), check)

	assert.Empty(t, check.GetResults())
}

func TestValidatePluginInstallableReportsBrokenPlugin(t *testing.T) {
	cases := []struct {
		name string
		// breakPlugin changes the scaffolding so that the plugin is not installable.
		breakPlugin func(t *testing.T, extensionDir string)
		// identifier is the problem the validation has to report.
		identifier string
	}{
		{
			name: "plugin class is missing",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `"shopware-plugin-class": "Swag\\BasicExample\\SwagBasicExample"`, `"shopware-plugin-class": ""`)
			},
			identifier: "installable.plugin-class",
		},
		{
			name: "shopware/core is not required",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `"shopware/core": "~6.7.0"`, `"symfony/yaml": "^7.0"`)
			},
			identifier: "installable.shopware-core",
		},
		{
			name: "en-GB label is empty",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `"en-GB": "Skeleton plugin"`, `"en-GB": ""`)
			},
			identifier: "installable.label",
		},
		{
			name: "plugin class does not match the directory name",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `\\SwagBasicExample"`, `\\SwagOtherExample"`)
			},
			identifier: "installable.technical-name",
		},
		{
			name: "composer name is not derived from the plugin name",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `"name": "swag/basic-example"`, `"name": "swag/basic_example"`)
			},
			identifier: "installable.composer-name",
		},
		{
			name: "namespace is not derived from the plugin name",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `Swag\\BasicExample\\SwagBasicExample`, `SwagBasicExample\\SwagBasicExample`)
			},
			identifier: "installable.namespace",
		},
		{
			name: "namespace is not autoloaded",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "composer.json"), `"Swag\\BasicExample\\": "src/"`, `"Swag\\WrongExample\\": "src/"`)
			},
			identifier: "installable.autoload",
		},
		{
			name: "plugin class file does not exist",
			breakPlugin: func(t *testing.T, extensionDir string) {
				require.NoError(t, os.Remove(filepath.Join(extensionDir, "src", "SwagBasicExample.php")))
			},
			identifier: "installable.plugin-class-file",
		},
		{
			name: "plugin class file declares another namespace",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "src", "SwagBasicExample.php"), `namespace Swag\BasicExample;`, `namespace Swag\WrongExample;`)
			},
			identifier: "installable.plugin-class-namespace",
		},
		{
			name: "plugin class does not extend the shopware plugin class",
			breakPlugin: func(t *testing.T, extensionDir string) {
				replaceInFile(t, filepath.Join(extensionDir, "src", "SwagBasicExample.php"), "class SwagBasicExample extends Plugin", "class SwagBasicExample")
			},
			identifier: "installable.plugin-base-class",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			extensionDir := scaffoldPlugin(t, "SwagBasicExample")
			testCase.breakPlugin(t, extensionDir)

			check := &testCheck{}
			validatePluginInstallable(loadPlugin(t, extensionDir), check)

			var identifiers []string
			for _, result := range check.GetResults() {
				identifiers = append(identifiers, result.Identifier)
			}

			assert.Contains(t, identifiers, testCase.identifier)
		})
	}
}

// scaffoldPlugin creates the files of a new plugin in a temporary directory.
func scaffoldPlugin(t *testing.T, name string) string {
	t.Helper()

	extensionDir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.Mkdir(extensionDir, 0o755))
	require.NoError(t, scaffolding.CreateScaffoldingFiles(extensionDir, name))

	return extensionDir
}

// loadPlugin reads an extension directory the same way the create command does.
func loadPlugin(t *testing.T, extensionDir string) *PlatformPlugin {
	t.Helper()

	ext, err := GetExtensionByFolder(t.Context(), extensionDir)
	require.NoError(t, err)

	plugin, ok := ext.(*PlatformPlugin)
	require.True(t, ok, "%s is not a platform plugin", extensionDir)

	return plugin
}

// replaceInFile rewrites a file with all occurrences of old replaced by new.
func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), old)

	require.NoError(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(content), old, replacement)), 0o644))
}
