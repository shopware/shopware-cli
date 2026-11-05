package verifier

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

func runCookieListenerWiring(t *testing.T, fixture string) (string, validation.ToolReport) {
	t.Helper()

	tmpDir := t.TempDir()
	require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join("testdata", "cookie-listener-wiring", fixture))))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)

	result := runFixTool(t, CookieListenerWiring{}, ToolConfig{
		Extensions:        []extension.Extension{ext},
		RootDir:           tmpDir,
		SourceDirectories: []string{filepath.Join(tmpDir, "src")},
	})
	require.Empty(t, result.Errors)

	return tmpDir, result
}

func assertFixtureFile(t *testing.T, expected, actual string) {
	t.Helper()

	want, err := os.ReadFile(expected)
	require.NoError(t, err)
	got, err := os.ReadFile(actual)
	require.NoError(t, err)
	assert.Equal(t, string(want), string(got))
}

func TestCookieListenerWiringIsAFollowUpOfRector(t *testing.T) {
	assert.Equal(t, ToolList[FixTool]{CookieListenerWiring{}}, FollowUpFixTools(ToolList[FixTool]{SymfonyXMLConverter{}, Rector{}}))
	assert.Empty(t, FollowUpFixTools(ToolList[FixTool]{SymfonyXMLConverter{}}))

	_, err := GetTools().Only(CookieListenerWiring{}.Name())
	assert.Error(t, err, "the wiring must not run in parallel with the symfony-xml fixer")
}

func TestCookieListenerWiringUpdatesServicesXML(t *testing.T) {
	tmpDir, result := runCookieListenerWiring(t, "xml")

	assert.Equal(t, "1 file updated", result.Note)
	assert.Empty(t, result.Findings)
	assertFixtureFile(t, filepath.Join("testdata", "cookie-listener-wiring", "xml", "expected-services.xml"), filepath.Join(tmpDir, "src", "Resources", "config", "services.xml"))
}

func TestCookieListenerWiringUpdatesServicesYAML(t *testing.T) {
	tmpDir, result := runCookieListenerWiring(t, "yaml")

	assert.Equal(t, "1 file updated", result.Note)
	assert.Empty(t, result.Findings)
	assertFixtureFile(t, filepath.Join("testdata", "cookie-listener-wiring", "yaml", "expected-services.yaml"), filepath.Join(tmpDir, "src", "Resources", "config", "services.yaml"))
}

func TestCookieListenerWiringReportsListenersWithoutDecoration(t *testing.T) {
	tmpDir, _ := runCookieListenerWiring(t, "yaml")
	require.NoError(t, os.Remove(filepath.Join(tmpDir, "src", "Resources", "config", "services.yaml")))

	result := runFixTool(t, CookieListenerWiring{}, ToolConfig{RootDir: tmpDir, SourceDirectories: []string{filepath.Join(tmpDir, "src")}})

	assert.Equal(t, "no changes", result.Note)
	require.Len(t, result.Findings, 2)
	for _, finding := range result.Findings {
		assert.Equal(t, "cookie-listener-wiring/not-found", finding.Identifier)
	}
}

func TestCookieListenerWiringSkipsYAMLFlowStyle(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join("testdata", "cookie-listener-wiring", "yaml"))))
	servicesFile := filepath.Join(tmpDir, "src", "Resources", "config", "services.yaml")
	services := "services:\n    Test\\TestPlugin\\Cookie\\AutowiredCookieProvider: { decorates: Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface }\n"
	require.NoError(t, os.WriteFile(servicesFile, []byte(services), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)
	result := runFixTool(t, CookieListenerWiring{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir, SourceDirectories: []string{filepath.Join(tmpDir, "src")}})

	content, err := os.ReadFile(servicesFile)
	require.NoError(t, err)
	assert.Equal(t, services, string(content))
	assert.ElementsMatch(t, []string{"cookie-listener-wiring/unsupported", "cookie-listener-wiring/not-found", "cookie-listener-wiring/not-found"}, []string{result.Findings[0].Identifier, result.Findings[1].Identifier, result.Findings[2].Identifier})
}

func TestCookieListenerWiringSkipsWithoutMigratedProvider(t *testing.T) {
	result := runFixTool(t, CookieListenerWiring{}, ToolConfig{RootDir: t.TempDir(), SourceDirectories: []string{t.TempDir()}})

	assert.Equal(t, "no cookie provider was migrated", result.SkipReason)
}
