package extension

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/validation"
)

func TestValidateExtensionName(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"SwagBasicExample", "MyPlugin", "AcmePayPal", "Swag2Example"} {
		assert.NoError(t, ValidateName(valid), valid)
	}

	for _, invalid := range []string{
		"", "swagBasicExample", "Swag", "my-plugin", "My_Plugin",
		"My Plugin", "1Plugin", "Swag.Example",
	} {
		assert.Error(t, ValidateName(invalid), invalid)
	}
}

func TestValidateCreateOptions(t *testing.T) {
	t.Parallel()

	valid := CreateOptions{
		Name:  "SwagBasicExample",
		Usage: PrivateUsage,
	}
	require.NoError(t, validateCreateOptions(valid))

	tests := []struct {
		name   string
		change func(*CreateOptions)
		error  string
	}{
		{
			name:   "invalid name",
			change: func(opts *CreateOptions) { opts.Name = "invalid-name" },
			error:  "invalid extension name",
		},
		{
			name:   "invalid usage",
			change: func(opts *CreateOptions) { opts.Usage = "unknown" },
			error:  "invalid extension usage",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := valid
			test.change(&opts)
			assert.ErrorContains(t, validateCreateOptions(opts), test.error)
		})
	}
}

func TestCreate(t *testing.T) {
	for _, usage := range []ExtensionUsage{PrivateUsage, CommercialUsage} {
		t.Run(string(usage), func(t *testing.T) {
			projectDir := prepareProject(t)
			opts := validCreateOptions()
			opts.Usage = usage

			require.NoError(t, Create(system.WithInteraction(t.Context(), false), opts))

			extensionDir := extensionDirectory(projectDir, opts)
			assert.FileExists(t, filepath.Join(extensionDir, "composer.json"))
			assert.FileExists(t, filepath.Join(extensionDir, "src", opts.Name+".php"))
			require.NoError(t, validateCreatedExtension(t.Context(), extensionDir))
		})
	}
}

func TestCreateRejectsInvalidOptionsBeforeCreatingFiles(t *testing.T) {
	projectDir := prepareProject(t)
	opts := validCreateOptions()
	opts.Name = "invalid-name"

	err := Create(system.WithInteraction(t.Context(), false), opts)

	assert.ErrorContains(t, err, "invalid extension name")
	assert.NoDirExists(t, extensionDirectory(projectDir, opts))
}

func TestCreateRequiresName(t *testing.T) {
	err := Create(t.Context(), CreateOptions{
		Usage: PrivateUsage,
	})

	assert.EqualError(t, err, "extension name must not be empty")
}

func TestCreateValidatesEnumsBeforeRequiringName(t *testing.T) {
	err := Create(system.WithInteraction(context.Background(), false), CreateOptions{
		Usage: "unknown",
	})

	assert.ErrorContains(t, err, "invalid extension usage")
}

func TestCreateFindsClosestProject(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "bin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "bin", "console"), nil, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(projectDir, "composer.json"),
		[]byte(`{"require":{"shopware/core":"~6.7.0"}}`),
		0o644,
	))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))
	nestedDir := filepath.Join(projectDir, "custom")
	t.Chdir(nestedDir)

	opts := validCreateOptions()
	require.NoError(t, Create(system.WithInteraction(t.Context(), false), opts))

	assert.FileExists(t, filepath.Join(extensionDirectory(projectDir, opts), "composer.json"))
}

func TestCreateFailsOutsideShopwareProject(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	t.Chdir(t.TempDir())

	err := Create(system.WithInteraction(t.Context(), false), validCreateOptions())

	assert.ErrorContains(t, err, "cannot find Shopware project")
}

func TestCreateReportsMissingExtensionParent(t *testing.T) {
	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	opts := validCreateOptions()

	err := Create(system.WithInteraction(t.Context(), false), opts)

	assert.ErrorContains(t, err, "extension parent directory does not exist")
	assert.NoDirExists(t, extensionDirectory(projectDir, opts))
}

func TestCreateDoesNotRemoveExistingDirectory(t *testing.T) {
	projectDir := prepareProject(t)
	opts := validCreateOptions()
	extensionDir := extensionDirectory(projectDir, opts)
	require.NoError(t, os.Mkdir(extensionDir, 0o755))
	marker := filepath.Join(extensionDir, "keep.txt")
	require.NoError(t, os.WriteFile(marker, []byte("keep"), 0o644))

	err := Create(system.WithInteraction(t.Context(), false), opts)

	assert.ErrorContains(t, err, "already exists")
	assert.FileExists(t, marker)
}

func TestValidateCreatedExtensionReportsDetails(t *testing.T) {
	extensionDir := scaffoldPlugin(t, "SwagBasicExample")
	mutateComposer(t, extensionDir, func(composer *PlatformComposerJson) {
		composer.Extra.Label["en-GB"] = ""
	})

	err := validateCreatedExtension(t.Context(), extensionDir)

	assert.ErrorContains(t, err, "installable.label")
	assert.ErrorContains(t, err, "extra.label")
}

func TestValidatePluginInstallable(t *testing.T) {
	extensionDir := scaffoldPlugin(t, "SwagBasicExample")
	check := &testCheck{}

	validatePluginInstallable(loadPlugin(t, extensionDir), check)

	assert.Empty(t, check.GetResults())
}

func TestValidatePluginInstallableReportsBrokenPlugin(t *testing.T) {
	tests := []struct {
		name        string
		breakPlugin func(*testing.T, string)
		identifiers []string
	}{
		{
			name: "plugin class is missing",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) { c.Extra.ShopwarePluginClass = "" })
			},
			identifiers: []string{"installable.plugin-class"},
		},
		{
			name: "shopware core requirement is missing",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) { delete(c.Require, "shopware/core") })
			},
			identifiers: []string{"installable.shopware-core"},
		},
		{
			name: "shopware core constraint is invalid",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) { c.Require["shopware/core"] = "not a version" })
			},
			identifiers: []string{"installable.shopware-core"},
		},
		{
			name: "English label is empty",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) { c.Extra.Label["en-GB"] = "" })
			},
			identifiers: []string{"installable.label"},
		},
		{
			name: "technical name differs",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) {
					c.Extra.ShopwarePluginClass = `Swag\BasicExample\SwagOtherExample`
				})
			},
			identifiers: []string{"installable.technical-name", "installable.plugin-class-file"},
		},
		{
			name: "composer name differs",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) { c.Name = "swag/basic_example" })
			},
			identifiers: []string{"installable.composer-name"},
		},
		{
			name: "namespace differs",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) {
					c.Extra.ShopwarePluginClass = `Swag\OtherExample\SwagBasicExample`
					c.Autoload.Psr4 = map[string]string{`Swag\OtherExample\`: "src/"}
				})
			},
			identifiers: []string{"installable.namespace", "installable.plugin-class-namespace"},
		},
		{
			name: "namespace is not autoloaded",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) {
					c.Autoload.Psr4 = map[string]string{`Swag\WrongExample\`: "src/"}
				})
			},
			identifiers: []string{"installable.autoload"},
		},
		{
			name: "class file is missing",
			breakPlugin: func(t *testing.T, dir string) {
				require.NoError(t, os.Remove(filepath.Join(dir, "src", "SwagBasicExample.php")))
			},
			identifiers: []string{"installable.plugin-class-file"},
		},
		{
			name: "class file namespace differs",
			breakPlugin: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "src", "SwagBasicExample.php"),
					`namespace Swag\BasicExample;`, `namespace Swag\WrongExample;`)
			},
			identifiers: []string{"installable.plugin-class-namespace"},
		},
		{
			name: "class name differs",
			breakPlugin: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "src", "SwagBasicExample.php"),
					"class SwagBasicExample extends Plugin", "class OtherClass extends Plugin")
			},
			identifiers: []string{"installable.plugin-class-file"},
		},
		{
			name: "base class is missing",
			breakPlugin: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "src", "SwagBasicExample.php"),
					"class SwagBasicExample extends Plugin", "class SwagBasicExample")
			},
			identifiers: []string{"installable.plugin-base-class"},
		},
		{
			name: "multiple metadata errors",
			breakPlugin: func(t *testing.T, dir string) {
				mutateComposer(t, dir, func(c *PlatformComposerJson) {
					delete(c.Require, "shopware/core")
					c.Extra.Label["en-GB"] = ""
					c.Name = "wrong/name"
				})
			},
			identifiers: []string{
				"installable.shopware-core",
				"installable.label",
				"installable.composer-name",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extensionDir := scaffoldPlugin(t, "SwagBasicExample")
			test.breakPlugin(t, extensionDir)
			check := &testCheck{}

			validatePluginInstallable(loadPlugin(t, extensionDir), check)

			assert.ElementsMatch(t, test.identifiers, resultIdentifiers(check.GetResults()))
		})
	}
}

func TestPluginClassFileSupportsPSR0(t *testing.T) {
	extensionDir := scaffoldPlugin(t, "SwagBasicExample")
	oldClassFile := filepath.Join(extensionDir, "src", "SwagBasicExample.php")
	newClassFile := filepath.Join(extensionDir, "src", "Swag", "BasicExample", "SwagBasicExample.php")
	require.NoError(t, os.MkdirAll(filepath.Dir(newClassFile), 0o755))
	require.NoError(t, os.Rename(oldClassFile, newClassFile))
	mutateComposer(t, extensionDir, func(c *PlatformComposerJson) {
		c.Autoload.Psr4 = nil
		c.Autoload.Psr0 = map[string]string{`Swag\BasicExample\`: "src/"}
	})
	check := &testCheck{}

	validatePluginInstallable(loadPlugin(t, extensionDir), check)

	assert.Empty(t, check.GetResults())
}

func TestPluginClassMayExtendFullyQualifiedBaseClass(t *testing.T) {
	extensionDir := scaffoldPlugin(t, "SwagBasicExample")
	replaceInFile(t, filepath.Join(extensionDir, "src", "SwagBasicExample.php"),
		"class SwagBasicExample extends Plugin",
		`class SwagBasicExample extends \Shopware\Core\Framework\Plugin`)
	check := &testCheck{}

	validatePluginInstallable(loadPlugin(t, extensionDir), check)

	assert.Empty(t, check.GetResults())
}

func validCreateOptions() CreateOptions {
	return CreateOptions{
		Name:  "SwagBasicExample",
		Usage: PrivateUsage,
	}
}

func prepareProject(t *testing.T) string {
	t.Helper()

	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "plugins"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))
	return projectDir
}

func scaffoldPlugin(t *testing.T, name string) string {
	t.Helper()

	extensionDir := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.Mkdir(extensionDir, 0o755))
	require.NoError(t, scaffolding.CreateExtensionFiles(extensionDir, name))
	return extensionDir
}

func loadPlugin(t *testing.T, extensionDir string) *PlatformPlugin {
	t.Helper()

	ext, err := GetExtensionByFolder(t.Context(), extensionDir)
	require.NoError(t, err)
	plugin, ok := ext.(*PlatformPlugin)
	require.True(t, ok, "%s is not a platform plugin", extensionDir)
	return plugin
}

func mutateComposer(t *testing.T, extensionDir string, mutate func(*PlatformComposerJson)) {
	t.Helper()

	path := filepath.Join(extensionDir, "composer.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var composer PlatformComposerJson
	require.NoError(t, json.Unmarshal(content, &composer))
	mutate(&composer)

	content, err = json.MarshalIndent(composer, "", "    ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(content, '\n'), 0o644))
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), old)
	updated := strings.Replace(string(content), old, replacement, 1)
	require.NoError(t, os.WriteFile(path, []byte(updated), 0o644))
}

func resultIdentifiers(results []validation.CheckResult) []string {
	identifiers := make([]string, 0, len(results))
	for _, result := range results {
		identifiers = append(identifiers, result.Identifier)
	}
	return identifiers
}
