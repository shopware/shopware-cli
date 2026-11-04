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
	assert.Equal(t, "2 files converted", result.Note)

	second := runFixTool(t, SymfonyXMLConverter{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir})
	assert.Empty(t, second.Errors)
	assert.Empty(t, second.Findings)
	assert.Equal(t, "skipped", second.Status())
	assert.NotEmpty(t, second.SkipReason)
	assert.Empty(t, second.Note)
	assertConvertedConfigDir(t, filepath.Join(tmpDir, "src", "Resources", "config"))
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
			assert.Equal(t, "1 file converted", result.Note)
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
	assert.Equal(t, "1 file converted", result.Note)
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
	assert.Equal(t, "3 files converted", result.Note)
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
