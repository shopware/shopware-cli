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

func TestCookieListenerWiringUpdatesServicesXML(t *testing.T) {
	tmpDir, result := runCookieListenerWiring(t, "xml")

	assert.Equal(t, "1 file updated", result.Note)
	assert.Empty(t, result.Findings)
	assertFixtureFile(t, filepath.Join("testdata", "cookie-listener-wiring", "xml", "expected-services.xml"), filepath.Join(tmpDir, "src", "Resources", "config", "services.xml"))
	assertFixtureFile(t, filepath.Join("testdata", "cookie-listener-wiring", "xml", "src", "Resources", "config", "routes.xml"), filepath.Join(tmpDir, "src", "Resources", "config", "routes.xml"))
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

const testCookieListener = `Test\TestPlugin\Cookie\MigratedCookieProvider`

func TestCookieListenerWiringKeepsInlineServicesInXML(t *testing.T) {
	content := `<services>
    <service id="Test\TestPlugin\Cookie\MigratedCookieProvider" decorates="Shopware\Storefront\Framework\Cookie\CookieProviderInterface">
        <argument type="service">
            <service class="Test\TestPlugin\Inline">
                <argument type="service">
                    <service class="Test\TestPlugin\Nested"/>
                </argument>
            </service>
        </argument>
        <argument type="service" id=".inner"/>
    </service>
</services>`
	expected := `<services>
    <service id="Test\TestPlugin\Cookie\MigratedCookieProvider">
        <argument type="service">
            <service class="Test\TestPlugin\Inline">
                <argument type="service">
                    <service class="Test\TestPlugin\Nested"/>
                </argument>
            </service>
        </argument>
        <tag name="kernel.event_listener" event="Shopware\Core\Content\Cookie\Event\CookieGroupCollectEvent"/>
    </service>
</services>`

	wired := map[string]bool{}
	assert.Equal(t, expected, rewireCookieListenersXML(content, map[string]string{testCookieListener: "file.php"}, wired))
	assert.True(t, wired[testCookieListener])
}

func TestCookieListenerWiringKeepsUnclosedXMLService(t *testing.T) {
	content := `<service id="Test\TestPlugin\Cookie\MigratedCookieProvider" decorates="Shopware\Storefront\Framework\Cookie\CookieProviderInterface">`

	wired := map[string]bool{}
	assert.Equal(t, content, rewireCookieListenersXML(content, map[string]string{testCookieListener: "file.php"}, wired))
	assert.Empty(t, wired)
}

func TestCookieListenerWiringYAMLShapes(t *testing.T) {
	listeners := map[string]string{testCookieListener: "file.php"}

	cases := map[string]struct {
		content  string
		expected string
		wired    bool
	}{
		"comment only": {
			content:  "# Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface\n",
			expected: "# Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface\n",
		},
		"no services": {
			content:  "parameters:\n    provider: Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface\n",
			expected: "parameters:\n    provider: Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface\n",
		},
		"alias": {
			content:  "services:\n    Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface: '@test.provider'\n",
			expected: "services:\n    Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface: '@test.provider'\n",
		},
		"custom inner name": {
			content: "services:\n" +
				"    Test\\TestPlugin\\Cookie\\MigratedCookieProvider:\n" +
				"        decorates: Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface\n" +
				"        decoration_inner_name: test.cookie_provider.original\n" +
				"        arguments:\n" +
				"            - '@test.cookie_provider.original'\n",
			expected: "services:\n" +
				"    Test\\TestPlugin\\Cookie\\MigratedCookieProvider:\n" +
				"        tags:\n" +
				"            - { name: kernel.event_listener, event: Shopware\\Core\\Content\\Cookie\\Event\\CookieGroupCollectEvent }\n",
			wired: true,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			wired := map[string]bool{}
			updated, skipped, err := rewireCookieListenersYAML(tc.content, listeners, wired)

			require.NoError(t, err)
			assert.Empty(t, skipped)
			assert.Equal(t, tc.expected, updated)
			assert.Equal(t, tc.wired, wired[testCookieListener])
		})
	}
}

func TestCookieListenerWiringFailsOnInvalidYAML(t *testing.T) {
	tmpDir, _ := runCookieListenerWiring(t, "yaml")
	servicesFile := filepath.Join(tmpDir, "src", "Resources", "config", "services.yaml")
	require.NoError(t, os.WriteFile(servicesFile, []byte("services: [Shopware\\Storefront\\Framework\\Cookie\\CookieProviderInterface\n"), 0o644))

	ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
	require.NoError(t, err)
	result := runFixTool(t, CookieListenerWiring{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir, SourceDirectories: []string{filepath.Join(tmpDir, "src")}})

	require.Len(t, result.Errors, 1)
	assert.ErrorContains(t, result.Errors[0].Err, "cannot update")
}

func TestFindCookieListenersSkipsDependenciesAndAnonymousClasses(t *testing.T) {
	tmpDir := t.TempDir()
	listener := "<?php\nnamespace Test;\nclass Listener\n{\n    public function __invoke(CookieGroupCollectEvent $event): void {}\n}\n"
	anonymous := "<?php\nreturn new class {\n    public function __invoke(CookieGroupCollectEvent $event): void {}\n};\n"
	for path, content := range map[string]string{
		"Listener.php":           listener,
		"anonymous.php":          anonymous,
		"vendor/Listener.php":    listener,
		"node_modules/other.php": listener,
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(tmpDir, path)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, path), []byte(content), 0o644))
	}

	listeners, err := findCookieListeners([]string{tmpDir, filepath.Join(tmpDir, "missing")})

	require.NoError(t, err)
	assert.Equal(t, map[string]string{`Test\Listener`: filepath.Join(tmpDir, "Listener.php")}, listeners)
}

func TestCookieListenerWiringFailsOnUnreadableFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}

	cases := map[string]struct {
		path      string
		createDir bool
		mode      os.FileMode
		sources   string
		message   string
	}{
		"unreadable source directory": {
			path: filepath.Join("src", "Locked"), createDir: true, mode: 0o000,
			sources: "src", message: "cannot search cookie listeners",
		},
		"unreadable source file": {
			path: filepath.Join("src", "Cookie", "MigratedCookieProvider.php"), mode: 0o000,
			sources: "src", message: "cannot search cookie listeners",
		},
		"unreadable configuration directory": {
			path: filepath.Join("src", "Resources", "config", "packages"), createDir: true, mode: 0o000,
			sources: filepath.Join("src", "Cookie"), message: "cannot search service configuration",
		},
		"unreadable configuration file": {
			path: filepath.Join("src", "Resources", "config", "services.yaml"), mode: 0o000,
			sources: filepath.Join("src", "Cookie"), message: "cannot read service configuration",
		},
		"read-only configuration file": {
			path: filepath.Join("src", "Resources", "config", "services.yaml"), mode: 0o444,
			sources: filepath.Join("src", "Cookie"), message: "cannot write service configuration",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			tmpDir := t.TempDir()
			require.NoError(t, os.CopyFS(tmpDir, os.DirFS(filepath.Join("testdata", "cookie-listener-wiring", "yaml"))))
			ext, err := extension.GetExtensionByFolder(t.Context(), tmpDir)
			require.NoError(t, err)

			path := filepath.Join(tmpDir, tc.path)
			if tc.createDir {
				require.NoError(t, os.MkdirAll(path, 0o755))
			}
			info, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.Chmod(path, tc.mode))
			t.Cleanup(func() { _ = os.Chmod(path, info.Mode().Perm()) })

			result := runFixTool(t, CookieListenerWiring{}, ToolConfig{Extensions: []extension.Extension{ext}, RootDir: tmpDir, SourceDirectories: []string{filepath.Join(tmpDir, tc.sources)}})

			require.Len(t, result.Errors, 1)
			assert.ErrorContains(t, result.Errors[0].Err, tc.message)
		})
	}
}
