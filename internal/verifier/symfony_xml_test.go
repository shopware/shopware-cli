package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

// copySymfonyXMLFixture copies testdata/symfony-xml/<name> into a temporary
// directory, since the fixer modifies the files in place.
func copySymfonyXMLFixture(t *testing.T, name string) string {
	t.Helper()

	tmpDir := t.TempDir()
	require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join("testdata", "symfony-xml", name))))

	return tmpDir
}

func expectedFixtureYAML(t *testing.T, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join("testdata", "symfony-xml", name))
	require.NoError(t, err)

	return string(content)
}

func assertConvertedConfigDir(t *testing.T, configDir string) {
	t.Helper()

	assert.NoFileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "routes.xml"))

	services, err := os.ReadFile(filepath.Join(configDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedFixtureYAML(t, "expected-services.yaml"), string(services))

	routes, err := os.ReadFile(filepath.Join(configDir, "routes.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedFixtureYAML(t, "expected-routes.yaml"), string(routes))
}

func TestSymfonyXMLConverterIsRegistered(t *testing.T) {
	_, err := GetToolsOf[FixTool]().Only("symfony-xml")
	assert.NoError(t, err)
}

func TestSymfonyXMLConverterFixesPlugin(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "plugin")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir})
	require.Empty(t, result.Errors)

	assertConvertedConfigDir(t, filepath.Join(tmpDir, "src", "Resources", "config"))
	assert.Empty(t, result.Findings)
	assert.Len(t, result.ChangedFiles, 2)
	assert.Equal(t, []string{"src/Resources/config/routes.xml", "src/Resources/config/services.xml"}, result.ChangedFiles)

	second := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir})
	assert.Empty(t, second.Errors)
	assert.Empty(t, second.Findings)
	assert.Equal(t, "skipped", second.Status())
	assert.NotEmpty(t, second.SkipReason)
	assert.Empty(t, second.Note)
	assert.Empty(t, second.ChangedFiles)
	assertConvertedConfigDir(t, filepath.Join(tmpDir, "src", "Resources", "config"))
}

func TestSymfonyXMLConverterFindingMessagesThroughSymlinkedRoot(t *testing.T) {
	root := copySymfonyXMLFixture(t, "unconvertible")
	alias := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, os.Symlink(root, alias))
	ext, err := extension.GetExtensionByFolder(t.Context(), alias)
	require.NoError(t, err)
	for _, dryRun := range []bool{false, true} {
		result := runFixToolWithDryRun(t, SymfonyXMLConverter{}, ToolConfig{
			RootDir: alias, Extensions: []extension.Extension{ext},
		}, dryRun)
		require.Empty(t, result.Errors)
		require.NotEmpty(t, result.Findings)
		for _, finding := range result.Findings {
			assert.Contains(t, finding.Message, "cannot convert src/Resources/config/")
			assert.NotContains(t, finding.Message, alias)
			assert.NotContains(t, finding.Message, root)
		}
	}
}

func TestSymfonyXMLConverterFixesExtraBundles(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "plugin-extra-bundle")
	bundleConfigDir := filepath.Join(tmpDir, "src", "Foo", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir})
	require.Empty(t, result.Errors)

	assert.NoFileExists(t, filepath.Join(bundleConfigDir, "services.xml"))

	content, err := os.ReadFile(filepath.Join(bundleConfigDir, "services.yaml"))
	require.NoError(t, err)
	assert.Equal(t, expectedFixtureYAML(t, "expected-services.yaml"), string(content))
}

func TestSymfonyXMLConverterSkipsApps(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "app")
	configDir := filepath.Join(tmpDir, "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir})
	require.Empty(t, result.Errors)

	assert.Equal(t, "skipped", result.Status())
	assert.Empty(t, result.Findings)
	assert.Empty(t, result.Note)
	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.FileExists(t, filepath.Join(configDir, "routes.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
	assert.NoFileExists(t, filepath.Join(configDir, "routes.yaml"))
}

func TestSymfonyXMLConverterKeepsUnconvertibleFile(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "unconvertible")
	configDir := filepath.Join(tmpDir, "src", "Resources", "config")

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir})
	require.Empty(t, result.Errors)
	require.Len(t, result.Findings, 1)
	assert.Equal(t, validation.SeverityError, result.Findings[0].Severity)
	assert.Equal(t, "symfony-xml/unsupported", result.Findings[0].Identifier)

	assert.FileExists(t, filepath.Join(configDir, "services.xml"))
	assert.NoFileExists(t, filepath.Join(configDir, "services.yaml"))
}

func TestSymfonyXMLConverterReportsBlockedConversions(t *testing.T) {
	const imports = `<container><imports><import resource="broken.xml"/></imports></container>`
	for _, scenario := range []struct {
		name, services, imported, target, findingPath, identifier string
		line                                                      int
	}{
		{"invalid XML", "<container", "", "", "services.xml", "invalid-xml", 1},
		{"existing YAML", "<container/>", "", "services.yaml", "services.xml", "target-exists", 1},
		{"existing YML", "<container/>", "", "services.yml", "services.xml", "target-exists", 1},
		{"unsupported import", imports, `<container><services><stack id="foo"/></services></container>`, "", "broken.xml", "unsupported", 1},
		{"invalid import", imports, "<container", "", "broken.xml", "invalid-xml", 1},
		{"import target conflict", imports, "<container/>", "broken.yaml", "broken.xml", "target-exists", 1},
		{"multiline invalid import", imports, "<container>\n<services>\n<service></services>\n</container>", "", "broken.xml", "invalid-xml", 3},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := copySymfonyXMLFixture(t, "plugin")
			directory := filepath.Join(root, "src", "Resources", "config")
			servicesPath := filepath.Join(directory, "services.xml")
			require.NoError(t, os.WriteFile(servicesPath, []byte(scenario.services), 0o644))
			if scenario.imported != "" {
				require.NoError(t, os.WriteFile(filepath.Join(directory, "broken.xml"), []byte(scenario.imported), 0o644))
			}
			if scenario.target != "" {
				require.NoError(t, os.WriteFile(filepath.Join(directory, scenario.target), []byte("existing target\n"), 0o644))
			}
			ext, err := extension.GetExtensionByFolder(t.Context(), root)
			require.NoError(t, err)
			// Duplicate discovery must not publish the same blocked conversion twice.
			result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{RootDir: root, Extensions: []extension.Extension{ext, ext}})
			require.Empty(t, result.Errors)
			require.Len(t, result.Findings, 1)
			assert.Equal(t, "src/Resources/config/"+scenario.findingPath, result.Findings[0].Path)
			assert.Equal(t, "symfony-xml/"+scenario.identifier, result.Findings[0].Identifier)
			assert.Equal(t, validation.SeverityError, result.Findings[0].Severity)
			assert.Equal(t, scenario.line, result.Findings[0].Line)
			assert.NotEmpty(t, result.Findings[0].Message)
			assert.NotEmpty(t, result.Findings[0].Tip)
			assert.True(t, result.HasFindings)
			assert.Len(t, result.ChangedFiles, 1)
			services, err := os.ReadFile(servicesPath)
			require.NoError(t, err)
			assert.Equal(t, scenario.services, string(services))
			assert.NoFileExists(t, filepath.Join(directory, "routes.xml"))
			assert.FileExists(t, filepath.Join(directory, "routes.yaml"))
			if scenario.imported != "" {
				imported, err := os.ReadFile(filepath.Join(directory, "broken.xml"))
				require.NoError(t, err)
				assert.Equal(t, scenario.imported, string(imported))
			}
			if scenario.target != "" {
				target, err := os.ReadFile(filepath.Join(directory, scenario.target))
				require.NoError(t, err)
				assert.Equal(t, "existing target\n", string(target))
			}
		})
	}
}

func TestSymfonyXMLConverterReportsFilesystemFailure(t *testing.T) {
	root := copySymfonyXMLFixture(t, "plugin")
	directory := filepath.Join(root, "src", "Resources", "config")
	// A directory at the source path fails deterministically, including as root.
	require.NoError(t, os.Remove(filepath.Join(directory, "services.xml")))
	require.NoError(t, os.Mkdir(filepath.Join(directory, "services.xml"), 0o755))
	ext, err := extension.GetExtensionByFolder(t.Context(), root)
	require.NoError(t, err)
	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{RootDir: root, Extensions: []extension.Extension{ext}})
	require.Len(t, result.Errors, 1)
	var pathError *os.PathError
	assert.ErrorAs(t, result.Errors[0], &pathError)
	assert.Empty(t, result.Findings)
	assert.Len(t, result.ChangedFiles, 1)
	assert.FileExists(t, filepath.Join(directory, "routes.yaml"))
}

func TestSymfonyXMLConverterFixesProjectPlugins(t *testing.T) {
	tmpDir := copySymfonyXMLFixture(t, "project")

	exts, err := extension.FindExtensionsFromProject(t.Context(), tmpDir, true)
	require.NoError(t, err)
	require.NotEmpty(t, exts)
	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{RootDir: tmpDir, Extensions: exts})
	require.Empty(t, result.Errors)

	assertConvertedConfigDir(t, filepath.Join(tmpDir, "custom", "plugins", "TestPlugin", "src", "Resources", "config"))
}

func TestSymfonyXMLConverterCountsImportedFiles(t *testing.T) {
	root := copySymfonyXMLFixture(t, "plugin")
	directory := filepath.Join(root, "src", "Resources", "config")
	servicesPath := filepath.Join(directory, "services.xml")
	childPath := filepath.Join(directory, "child.xml")
	require.NoError(t, os.WriteFile(servicesPath, []byte(`<container><imports><import resource="child.xml"/></imports></container>`), 0o644))
	require.NoError(t, os.WriteFile(childPath, []byte(`<container/>`), 0o644))
	ext, err := extension.GetExtensionByFolder(t.Context(), root)
	require.NoError(t, err)

	result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{RootDir: root, Extensions: []extension.Extension{ext}})
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Len(t, result.ChangedFiles, 3)
	assert.Equal(t, []string{"src/Resources/config/child.xml", "src/Resources/config/routes.xml", "src/Resources/config/services.xml"}, result.ChangedFiles)
	assert.NoFileExists(t, servicesPath)
	assert.NoFileExists(t, childPath)
	assert.NoFileExists(t, filepath.Join(directory, "routes.xml"))
	assert.FileExists(t, filepath.Join(directory, "child.yaml"))
	assert.FileExists(t, filepath.Join(directory, "routes.yaml"))

	services, err := os.ReadFile(filepath.Join(directory, "services.yaml"))
	require.NoError(t, err)
	var config struct {
		Imports []struct {
			Resource string `yaml:"resource"`
		} `yaml:"imports"`
	}
	require.NoError(t, yaml.Unmarshal(services, &config))
	require.Len(t, config.Imports, 1)
	assert.Equal(t, "child.yaml", config.Imports[0].Resource)
}

func TestSymfonyXMLConverterFailureDoesNotClaimNoChanges(t *testing.T) {
	for _, failure := range []string{"stat", "read"} {
		t.Run(failure, func(t *testing.T) {
			root := copySymfonyXMLFixture(t, "plugin")
			directory := filepath.Join(root, "src", "Resources", "config")
			source := filepath.Join(directory, "services.xml")
			require.NoError(t, os.Remove(source))
			require.NoError(t, os.Remove(filepath.Join(directory, "routes.xml")))
			if failure == "stat" {
				require.NoError(t, os.Symlink("services.xml", source))
			} else {
				require.NoError(t, os.Mkdir(source, 0o755))
			}
			ext, err := extension.GetExtensionByFolder(t.Context(), root)
			require.NoError(t, err)

			result := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{RootDir: root, Extensions: []extension.Extension{ext}})
			require.Len(t, result.Errors, 1)
			var pathErr *os.PathError
			require.ErrorAs(t, result.Errors[0], &pathErr)
			assert.Equal(t, source, pathErr.Path)
			assert.Equal(t, "failed", result.Status())
			assert.Empty(t, result.Findings)
			assert.Empty(t, result.SkipReason)
			assert.Empty(t, result.Note)
			assert.NoFileExists(t, filepath.Join(directory, "services.yaml"))
		})
	}
}

func TestSymfonyXMLConverterDryRunPlansWithoutWriting(t *testing.T) {
	root := copySymfonyXMLFixture(t, "plugin")
	directory := filepath.Join(root, "src", "Resources", "config")
	servicesPath := filepath.Join(directory, "services.xml")
	require.NoError(t, os.WriteFile(servicesPath, []byte(`<container><imports><import resource="child.xml"/></imports></container>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "child.xml"), []byte(`<container/>`), 0o644))
	originals := make(map[string][]byte)
	for _, name := range []string{"services", "routes", "child"} {
		data, err := os.ReadFile(filepath.Join(directory, name+".xml"))
		require.NoError(t, err)
		originals[name] = data
	}
	ext, err := extension.GetExtensionByFolder(t.Context(), root)
	require.NoError(t, err)
	// Repeated discovery and imported-file traversal must not duplicate proposals.
	config := ToolConfig{RootDir: root, Extensions: []extension.Extension{ext, ext}}
	var expected []validation.CheckResult
	for name := range originals {
		expected = append(expected, validation.CheckResult{
			Path: "src/Resources/config/" + name + ".xml", Line: 1,
			Severity: validation.SeverityError, Identifier: "symfony-xml/conversion",
			Message: "XML configuration can be converted to YAML. Run fix without dry-run to apply it.",
		})
	}
	for range 2 {
		result := runFixToolWithDryRun(t, SymfonyXMLConverter{}, config, true)
		assert.Empty(t, result.Errors)
		assert.Empty(t, result.Note)
		assert.ElementsMatch(t, expected, result.Findings)
		for name, original := range originals {
			data, err := os.ReadFile(filepath.Join(directory, name+".xml"))
			require.NoError(t, err)
			assert.Equal(t, original, data)
			assert.NoFileExists(t, filepath.Join(directory, name+".yaml"))
		}
	}
}

func TestSymfonyXMLConverterDryRunPreservesBlockedAndSiblingFiles(t *testing.T) {
	root := copySymfonyXMLFixture(t, "plugin")
	directory := filepath.Join(root, "src", "Resources", "config")
	source := filepath.Join(directory, "services.xml")
	original := []byte("<container>\n<services>\n<service></services>\n</container>")
	require.NoError(t, os.WriteFile(source, original, 0o644))
	ext, err := extension.GetExtensionByFolder(t.Context(), root)
	require.NoError(t, err)
	result := runFixToolWithDryRun(t, SymfonyXMLConverter{}, ToolConfig{RootDir: root, Extensions: []extension.Extension{ext}}, true)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Note)
	require.Len(t, result.Findings, 2)
	assert.ElementsMatch(t, []string{"symfony-xml/invalid-xml", "symfony-xml/conversion"}, []string{result.Findings[0].Identifier, result.Findings[1].Identifier})
	for _, finding := range result.Findings {
		assert.Equal(t, validation.SeverityError, finding.Severity)
		if finding.Identifier == "symfony-xml/invalid-xml" {
			assert.Equal(t, "src/Resources/config/services.xml", finding.Path)
			assert.Equal(t, 3, finding.Line)
		}
	}
	data, err := os.ReadFile(source)
	require.NoError(t, err)
	assert.Equal(t, original, data)
	assert.FileExists(t, filepath.Join(directory, "routes.xml"))
	assert.NoFileExists(t, filepath.Join(directory, "services.yaml"))
	assert.NoFileExists(t, filepath.Join(directory, "routes.yaml"))
}

func TestSymfonyXMLConverterDryRunCleanAndFilesystemFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "clean"
		if failed {
			name = "filesystem failure"
		}
		t.Run(name, func(t *testing.T) {
			root := copySymfonyXMLFixture(t, "plugin")
			directory := filepath.Join(root, "src", "Resources", "config")
			source := filepath.Join(directory, "services.xml")
			require.NoError(t, os.Remove(source))
			require.NoError(t, os.Remove(filepath.Join(directory, "routes.xml")))
			if failed {
				require.NoError(t, os.Mkdir(source, 0o755))
			}
			ext, err := extension.GetExtensionByFolder(t.Context(), root)
			require.NoError(t, err)
			result := runFixToolWithDryRun(t, SymfonyXMLConverter{}, ToolConfig{RootDir: root, Extensions: []extension.Extension{ext}}, true)
			assert.Empty(t, result.Findings)
			assert.Empty(t, result.Note)
			if failed {
				require.Len(t, result.Errors, 1)
				var pathErr *os.PathError
				require.ErrorAs(t, result.Errors[0], &pathErr)
				assert.Equal(t, source, pathErr.Path)
				assert.Equal(t, "failed", result.Status())
			} else {
				assert.Empty(t, result.Errors)
				assert.Equal(t, "skipped", result.Status())
			}
			assert.NoFileExists(t, filepath.Join(directory, "services.yaml"))
			assert.NoFileExists(t, filepath.Join(directory, "routes.yaml"))
		})
	}
}

func TestSymfonyXMLConverterConvertsOtherConfigFiles(t *testing.T) {
	root := copySymfonyXMLFixture(t, "plugin")
	directory := filepath.Join(root, "src", "Resources", "config")
	// Files imported from a services.php or loaded by Shopware are only
	// recognized by their namespace.
	files := map[string]string{
		"services/gateway.xml": `<container xmlns="http://symfony.com/schema/dic/services"><imports><import resource="shared.xml"/></imports></container>`,
		"services/shared.xml":  `<container><services><service id="Shop\Shared" class="Shop\Shared"/></services></container>`,
		"routes/admin.xml":     `<routes xmlns="http://symfony.com/schema/routing"><route id="admin" path="/admin"/></routes>`,
		"other.xml":            `<container/>`,
		"config.xml":           `<config xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"/>`,
	}
	for name, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(directory, name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644))
	}
	ext, err := extension.GetExtensionByFolder(t.Context(), root)
	require.NoError(t, err)
	config := ToolConfig{RootDir: root, Extensions: []extension.Extension{ext}}

	dryRun := runFixToolWithDryRun(t, SymfonyXMLConverter{}, config, true)
	assert.Empty(t, dryRun.Errors)
	assert.Len(t, dryRun.Findings, 5)

	result := runFixTool(t, SymfonyXMLConverter{}, config)
	assert.Empty(t, result.Errors)
	assert.Empty(t, result.Findings)
	assert.Equal(t, []string{
		"src/Resources/config/routes.xml",
		"src/Resources/config/routes/admin.xml",
		"src/Resources/config/services.xml",
		"src/Resources/config/services/gateway.xml",
		"src/Resources/config/services/shared.xml",
	}, result.ChangedFiles)

	gateway, err := os.ReadFile(filepath.Join(directory, "services", "gateway.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(gateway), "resource: shared.yaml")
	assert.FileExists(t, filepath.Join(directory, "services", "shared.yaml"))
	assert.FileExists(t, filepath.Join(directory, "routes", "admin.yaml"))
	assert.FileExists(t, filepath.Join(directory, "other.xml"))
	assert.FileExists(t, filepath.Join(directory, "config.xml"))
}
