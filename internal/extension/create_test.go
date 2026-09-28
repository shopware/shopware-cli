package extension

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

func TestDeriveTechnicalName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "MyExtension", deriveTechnicalName("MyExtension", ""))
	assert.Equal(t, "MyVendorMyExtension", deriveTechnicalName("MyExtension", "MyVendor"))
}
func TestDeriveExtensionDirectoryName(t *testing.T) {
	projectDir := newProject(t)

	path := filepath.Join(projectDir, "custom", "static-plugins", "MyExtension")
	pathStore := filepath.Join(projectDir, "custom", "plugins", "MyVendorMyExtension")

	assert.Equal(t, path, deriveExtensionDirectoryName(projectDir, false, "MyExtension"))
	assert.Equal(t, pathStore, deriveExtensionDirectoryName(projectDir, true, "MyVendorMyExtension"))
}

func TestValidateExtensionName(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"SwagBasicExample", "MyPlugin", "AcmePayPal", "Swag2Example", "Example"} {
		assert.NoError(t, ValidateName(valid), valid)
	}

	for _, invalid := range []string{
		"", "swagBasicExample", "my-plugin", "My_Plugin",
		"My Plugin", "1Plugin", "Swag.Example",
	} {
		assert.Error(t, ValidateName(invalid), invalid)
	}
}

func TestValidateVendorName(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"Vendor", "MyVendor", "VendorAG"} {
		assert.NoError(t, ValidateVendor(valid), valid)
	}

	for _, invalid := range []string{
		"", "vendor", "my-vendor", "My_Vendor",
		"My Vendor", "1Vendor", "Vendor.Example",
	} {
		assert.Error(t, ValidateVendor(invalid), invalid)
	}
}

func TestValidateExtensionType(t *testing.T) {
	t.Parallel()

	for _, valid := range []ExtensionType{Plugin, Theme} {
		assert.NoError(t, ValidateType(valid), valid)
	}

	for _, invalid := range []ExtensionType{"", "pluginx", "themey", "invalid"} {
		assert.Error(t, ValidateType(invalid), invalid)
	}
}

func TestCreateErrors(t *testing.T) {
	t.Run("unsupported extension type", func(t *testing.T) {
		opts := validCreateOptions()
		opts.Type = "not-a-type"

		assert.EqualError(t, Create(t.Context(), opts), `unsupported extension type "not-a-type"`)
	})

	t.Run("cannot find Shopware project", func(t *testing.T) {
		t.Setenv("PROJECT_ROOT", "")
		t.Chdir(t.TempDir())

		err := Create(system.WithInteraction(t.Context(), false), validCreateOptions())

		assert.ErrorContains(t, err, "cannot find Shopware project")
	})

	t.Run("extension directory already exists", func(t *testing.T) {
		projectDir := newProject(t)
		opts := validCreateOptions()
		extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, deriveTechnicalName(opts.Name, opts.Vendor))
		require.NoError(t, os.Mkdir(extensionDir, 0o755))

		assert.ErrorContains(t, Create(t.Context(), opts), "already exists")
	})

	t.Run("path exists as file", func(t *testing.T) {
		projectDir := newProject(t)
		opts := validCreateOptions()
		extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, deriveTechnicalName(opts.Name, opts.Vendor))
		require.NoError(t, os.WriteFile(extensionDir, nil, 0o644))

		assert.ErrorContains(t, Create(t.Context(), opts), "not a directory")
	})

	t.Run("plugin root does not exist", func(t *testing.T) {
		projectDir := newProject(t)
		opts := validCreateOptions()
		require.NoError(t, os.RemoveAll(filepath.Join(projectDir, "custom")))
		extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, deriveTechnicalName(opts.Name, opts.Vendor))

		assert.ErrorContains(t, Create(t.Context(), opts), "does not exist")
		assert.NoDirExists(t, extensionDir)
	})

	t.Run("parent path not a directory", func(t *testing.T) {
		projectDir := newProject(t)
		opts := validCreateOptions()
		parentPath := filepath.Join(projectDir, "custom", "static-plugins")
		require.NoError(t, os.RemoveAll(parentPath))
		require.NoError(t, os.WriteFile(parentPath, nil, 0o644))
		extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, deriveTechnicalName(opts.Name, opts.Vendor))

		assert.ErrorContains(t, Create(t.Context(), opts), "not a directory")
		assert.NoDirExists(t, extensionDir)
	})

	t.Run("extension files cannot be written", func(t *testing.T) {
		projectDir := newProject(t)
		opts := validCreateOptions()
		opts.Vendor = "V"
		// Directory name stays under NAME_MAX (255); the plugin class file does not.
		opts.Name = strings.Repeat("A", 251)
		extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, deriveTechnicalName(opts.Name, opts.Vendor))

		assert.ErrorContains(t, Create(t.Context(), opts), "create extension files:")
		assert.NoDirExists(t, extensionDir)
	})
}

func TestCreateGeneratesAnExtension(t *testing.T) {
	for _, extensionType := range []ExtensionType{Plugin, Theme} {
		for _, store := range []bool{false, true} {
			t.Run(fmt.Sprintf("type=%s/store=%t", extensionType, store), func(t *testing.T) {
				projectDir := newProject(t)
				opts := validCreateOptions()
				opts.Type = extensionType
				opts.Store = store

				require.NoError(t, Create(t.Context(), opts))

				technicalName := deriveTechnicalName(opts.Name, opts.Vendor)
				extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, technicalName)
				assert.FileExists(t, filepath.Join(extensionDir, "composer.json"))
				assert.FileExists(t, filepath.Join(extensionDir, "src", technicalName+".php"))

				if extensionType == Plugin {
					assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "config", "config.xml"))
					assert.FileExists(t, filepath.Join(extensionDir, ".gitignore"))
					assert.FileExists(t, filepath.Join(extensionDir, "phpunit.xml"))
					assert.FileExists(t, filepath.Join(extensionDir, "tests", "TestBootstrap.php"))
					assert.NoFileExists(t, filepath.Join(extensionDir, "src", "Resources", "theme.json"))
					return
				}

				assert.FileExists(t, filepath.Join(extensionDir, "src", "Resources", "theme.json"))
				assert.FileExists(t, filepath.Join(
					extensionDir,
					"src/Resources/app/storefront/src/scss/overrides.scss",
				))
				assert.NoFileExists(t, filepath.Join(extensionDir, "phpunit.xml"))
			})
		}
	}
}

func validCreateOptions() CreateOptions {
	return CreateOptions{
		Name:   "MyExtension",
		Vendor: "MyVendor",
		Type:   Plugin,
	}
}

func newProject(t *testing.T) string {
	t.Helper()

	projectDir := t.TempDir()
	t.Setenv("PROJECT_ROOT", projectDir)
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "plugins"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(projectDir, "custom", "static-plugins"), 0o755))
	return projectDir
}
