package scaffolding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveNamespace(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "MyExtension", DeriveNamespace("", "MyExtension"))
	assert.Equal(t, `MyVendor\MyExtension`, DeriveNamespace("MyVendor", "MyExtension"))
}

func TestDeriveComposerName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "my-extension/my-extension", DeriveComposerName("", "MyExtension"))
	assert.Equal(t, "my-vendor/my-extension", DeriveComposerName("MyVendor", "MyExtension"))
}

func TestDeriveClassName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "MyVendorMyExtension", DeriveClassName("MyVendor", "MyExtension"))
}

func TestSplitPascalCase(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"My", "Extension"}, splitPascalCase("MyExtension"))
	assert.Equal(t, []string{"M", "E"}, splitPascalCase("ME"))
}

// CreateExtensionDir should create a directory with the given name.
func TestCreateExtensionDirCreatesDirectoryWithGivenName(t *testing.T) {
	// Store extensions live in custom/plugins, project ones in custom/static-plugins.
	for _, pluginRoot := range []string{"plugins", "static-plugins"} {
		t.Run(pluginRoot, func(t *testing.T) {
			extensionDir := filepath.Join(newProject(t), "custom", pluginRoot, "MyExtension")

			require.NoError(t, CreateExtensionDir(extensionDir))

			info, err := os.Stat(extensionDir)
			require.NoError(t, err)
			assert.True(t, info.IsDir())
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		})
	}
}

func TestCreateExtensionDirErrors(t *testing.T) {
	t.Run("extension directory already exists", func(t *testing.T) {
		extensionDir := filepath.Join(newProject(t), "custom", "plugins", "MyExtension")
		require.NoError(t, CreateExtensionDir(extensionDir))

		assert.ErrorContains(t, CreateExtensionDir(extensionDir), "already exists")
	})

	t.Run("path exists as file", func(t *testing.T) {
		extensionDir := filepath.Join(newProject(t), "custom", "plugins", "MyExtension")
		require.NoError(t, os.WriteFile(extensionDir, nil, 0o644))

		assert.ErrorContains(t, CreateExtensionDir(extensionDir), "not a directory")
	})

	t.Run("plugin root does not exist", func(t *testing.T) {
		projectDir := newProject(t)
		require.NoError(t, os.RemoveAll(filepath.Join(projectDir, "custom")))
		extensionDir := filepath.Join(projectDir, "custom", "plugins", "MyExtension")

		assert.ErrorContains(t, CreateExtensionDir(extensionDir), "does not exist")
		assert.NoDirExists(t, extensionDir)
	})

	t.Run("parent path not a directory", func(t *testing.T) {
		projectDir := newProject(t)
		parentPath := filepath.Join(projectDir, "custom", "plugins", "MyVendor")
		require.NoError(t, os.WriteFile(parentPath, nil, 0o644))
		extensionDir := filepath.Join(parentPath, "MyExtension")

		assert.ErrorContains(t, CreateExtensionDir(extensionDir), "not a directory")
		assert.NoDirExists(t, extensionDir)
	})
}

func TestCreatePluginFiles(t *testing.T) {
	projectDir := newProject(t)
	technicalName := "MyVendorMyExtension"
	extensionDir := filepath.Join(projectDir, "custom", "plugins", technicalName)
	require.NoError(t, os.MkdirAll(extensionDir, 0o755))

	require.NoError(t, CreatePluginFiles(extensionDir, "MyExtension", "MyVendor"))

	assert.DirExists(t, filepath.Join(extensionDir, "tests"))

	// all expected files for an installable extension are created
	assert.FileExists(t, filepath.Join(extensionDir, "composer.json"))
	assert.FileExists(t, filepath.Join(extensionDir, ".gitignore"))
	assert.FileExists(t, filepath.Join(extensionDir, "phpunit.xml"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", technicalName+".php"))
	assert.FileExists(t, filepath.Join(extensionDir, "tests", "TestBootstrap.php"))
}

func TestCreateThemeFiles(t *testing.T) {
	projectDir := newProject(t)
	technicalName := "MyVendorMyExtension"
	assetName := "my-vendor-my-extension"
	extensionDir := filepath.Join(projectDir, "custom", "plugins", technicalName)
	require.NoError(t, os.MkdirAll(extensionDir, 0o755))

	require.NoError(t, CreateThemeFiles(extensionDir, "MyExtension", "MyVendor"))
	// all expected files for a storefront theme are created
	assert.FileExists(t, filepath.Join(extensionDir, "composer.json"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", technicalName+".php"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "theme.json"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "src", "scss", "overrides.scss"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "src", "scss", "base.scss"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "src", "assets", ".gitkeep"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "src", "main.js"))
	assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "dist", "storefront", "js", assetName, assetName+".js"))
	// all expected directories for a storefront theme are created
	assert.DirExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "src", "scss"))
	assert.DirExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "src", "assets"))
	assert.DirExists(t, filepath.Join(extensionDir, "src", "Resources", "app", "storefront", "dist", "storefront", "js", assetName))
}

func TestCreateFileWithScaffoldingErrors(t *testing.T) {
	t.Run("destination file already exists", func(t *testing.T) {
		extensionDir := filepath.Join(t.TempDir(), "MyVendorMyExtension")
		file := scaffoldingFile{Path: filepath.Join("src", "MyExtension.php"), StubPath: "stubs/plugin_class.php.tmpl"}
		data := createScaffoldingData("MyVendor", "MyExtension")

		require.NoError(t, createFileWithScaffolding(extensionDir, file, data))
		assert.ErrorContains(t, createFileWithScaffolding(extensionDir, file, data), "file exists")
	})

	t.Run("stub file does not exist", func(t *testing.T) {
		extensionDir := filepath.Join(t.TempDir(), "MyVendorMyExtension")
		file := scaffoldingFile{Path: filepath.Join("src", "MyExtension.php"), StubPath: "stubs/does_not_exist.tmpl"}
		data := createScaffoldingData("MyVendor", "MyExtension")

		assert.ErrorContains(t, createFileWithScaffolding(extensionDir, file, data), "stub")
	})
}

func TestCreateFileWithScaffolding(t *testing.T) {
	extensionDir := filepath.Join(t.TempDir(), "MyVendorMyExtension")
	file := scaffoldingFile{Path: filepath.Join("src", "MyExtension.php"), StubPath: "stubs/plugin_class.php.tmpl"}
	data := createScaffoldingData("MyVendor", "MyExtension")

	require.NoError(t, createFileWithScaffolding(extensionDir, file, data))
	// assert it also created the necessary subdirectories
	assert.DirExists(t, filepath.Join(extensionDir, "src"))
	assert.FileExists(t, filepath.Join(extensionDir, file.Path))
}

func TestRemoveCreatedExtensionDir(t *testing.T) {
	for _, pluginRoot := range []string{"plugins", "static-plugins"} {
		t.Run(pluginRoot, func(t *testing.T) {
			extensionDir := filepath.Join(newProject(t), "custom", pluginRoot, "MyExtension")
			require.NoError(t, CreateExtensionDir(extensionDir))
			require.NoError(t, os.WriteFile(filepath.Join(extensionDir, "composer.json"), nil, 0o644))

			// The directory and its content are gone.
			require.NoError(t, RemoveCreatedExtensionDir(extensionDir))
			assert.NoDirExists(t, extensionDir)

			// Removing an already absent extension is safe.
			require.NoError(t, RemoveCreatedExtensionDir(extensionDir))
		})
	}
}

func TestValidateRemovableExtensionDirErrors(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		assert.Error(t, validateRemovableExtensionDir(""))
		assert.Error(t, validateRemovableExtensionDir("   "))
	})

	t.Run("filesystem root", func(t *testing.T) {
		root := string(filepath.Separator)

		assert.Error(t, validateRemovableExtensionDir(root))
	})

	t.Run("not inside a plugin root", func(t *testing.T) {
		otherDir := filepath.Join(newProject(t), "custom", "apps", "MyExtension")
		require.NoError(t, os.MkdirAll(otherDir, 0o755))

		assert.ErrorContains(t, validateRemovableExtensionDir(otherDir), "not an extension directory")
	})

	t.Run("plugin root itself", func(t *testing.T) {
		pluginRoot := filepath.Join(newProject(t), "custom", "plugins")

		assert.ErrorContains(t, validateRemovableExtensionDir(pluginRoot), "not an extension directory")
	})

	t.Run("symlink", func(t *testing.T) {
		pluginRoot := filepath.Join(newProject(t), "custom", "plugins")
		target := filepath.Join(pluginRoot, "Target")
		link := filepath.Join(pluginRoot, "MyExtension")
		require.NoError(t, os.Mkdir(target, 0o755))
		require.NoError(t, os.Symlink(target, link))

		assert.ErrorContains(t, validateRemovableExtensionDir(link), "symlink")
	})

	t.Run("not a directory", func(t *testing.T) {
		file := filepath.Join(newProject(t), "custom", "plugins", "MyExtension")
		require.NoError(t, os.WriteFile(file, nil, 0o644))

		assert.ErrorContains(t, validateRemovableExtensionDir(file), "not a directory")
	})
}

func TestRemoveCreatedExtensionDirErrorsWhenRemovalFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may remove files in a read-only directory")
	}

	pluginRoot := filepath.Join(newProject(t), "custom", "plugins")
	extensionDir := filepath.Join(pluginRoot, "MyExtension")
	require.NoError(t, CreateExtensionDir(extensionDir))
	require.NoError(t, os.WriteFile(filepath.Join(extensionDir, "composer.json"), nil, 0o644))

	// A read-only plugin root makes the removal fail.
	require.NoError(t, os.Chmod(pluginRoot, 0o500))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(pluginRoot, 0o755))
	})

	err := RemoveCreatedExtensionDir(extensionDir)

	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "remove"), "unexpected error: %v", err)
	assert.DirExists(t, extensionDir)
}

// newProject creates an empty Shopware project with both plugin roots and
// returns the project directory.
func newProject(t *testing.T) string {
	t.Helper()

	projectDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "plugins"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))

	return projectDir
}
