package extension

import (
	"fmt"
	"os"
	"path/filepath"
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

func TestCreateFailsOutsideShopwareProject(t *testing.T) {
	t.Setenv("PROJECT_ROOT", "")
	t.Chdir(t.TempDir())

	err := Create(system.WithInteraction(t.Context(), false), validCreateOptions())

	assert.ErrorContains(t, err, "cannot find Shopware project")
}

func TestCreateGeneratesAnExtension(t *testing.T) {
	for _, store := range []bool{false, true} {
		t.Run(fmt.Sprintf("store=%t", store), func(t *testing.T) {
			projectDir := newProject(t)
			opts := validCreateOptions()
			opts.Store = store

			require.NoError(t, Create(t.Context(), opts))

			technicalName := deriveTechnicalName(opts.Name, opts.Vendor)
			extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, technicalName)
			assert.FileExists(t, filepath.Join(extensionDir, "composer.json"))
			assert.NoFileExists(t, filepath.Join(extensionDir, "src", "Resources", "config", "config.xml"))
			assert.FileExists(t, filepath.Join(extensionDir, ".gitignore"))
			assert.FileExists(t, filepath.Join(extensionDir, "phpunit.xml"))
			assert.FileExists(t, filepath.Join(extensionDir, "src", technicalName+".php"))
			assert.FileExists(t, filepath.Join(extensionDir, "tests", "TestBootstrap.php"))
		})
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
