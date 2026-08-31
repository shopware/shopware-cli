package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

// testAppManifestWith builds a variant of the complete MyExampleApp manifest.
func testAppManifestWith(mutate func(*testhelper.AppManifest)) testhelper.AppManifest {
	m := testhelper.NewAppManifest("MyExampleApp")
	mutate(&m)
	return m
}

var (
	testAppManifest                 = testhelper.NewAppManifest("MyExampleApp")
	testAppManifestMissingLicense   = testAppManifestWith(func(m *testhelper.AppManifest) { m.License = "" })
	testAppManifestMissingCopyright = testAppManifestWith(func(m *testhelper.AppManifest) { m.Copyright = "" })
	testAppManifestMissingAuthor    = testAppManifestWith(func(m *testhelper.AppManifest) { m.Author = "" })
	testAppManifestCompatibility    = testAppManifestWith(func(m *testhelper.AppManifest) { m.Compatibility = "~6.5.0" })
	testAppManifestIcon             = testAppManifestWith(func(m *testhelper.AppManifest) { m.Icon = "app.png" })
	testAppManifestSetup            = testAppManifestWith(func(m *testhelper.AppManifest) {
		m.Compatibility = "~6.5.0"
		m.SetupSecret = "foo"
	})
)

func TestIconNotExists(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifest)

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	assert.Equal(t, "MyExampleApp", app.manifest.Meta.Name)
	assert.Equal(t, "", app.manifest.Meta.Icon)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The extension icon Resources/config/plugin.png does not exist", check.Results[0].Message)
}

func TestAppNoLicense(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifestMissingLicense)
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The element meta:license was not found in the manifest.xml", check.Results[0].Message)
}

func TestAppNoCopyright(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifestMissingCopyright)
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The element meta:copyright was not found in the manifest.xml", check.Results[0].Message)
}

func TestAppNoAuthor(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifestMissingAuthor)
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The element meta:author was not found in the manifest.xml", check.Results[0].Message)
}

func TestAppHasSecret(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifestSetup)
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The xml element setup:secret is only for local development, please remove it. You can find your generated app secret on your extension detail page in the master data section. For more information see https://docs.shopware.com/en/shopware-platform-dev-en/app-system-guide/setup#authorisation", check.Results[0].Message)
}

func TestIconExistsDefaultsPath(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifest)

	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	assert.Equal(t, "MyExampleApp", app.manifest.Meta.Name)
	assert.Equal(t, "", app.manifest.Meta.Icon)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 0, len(check.Results))
}

func TestIconExistsDifferentPath(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifestIcon)
	assert.NoError(t, createTestImageWithSize(filepath.Join(appPath, "app.png"), 120, 120))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	assert.Equal(t, "MyExampleApp", app.manifest.Meta.Name)
	assert.Equal(t, "app.png", app.manifest.Meta.Icon)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 0, len(check.Results))
}

func TestNoCompatibilityGiven(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifest)

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	compatibility, err := app.GetShopwareVersionConstraint()
	assert.NoError(t, err)

	assert.Equal(t, "~6.4", compatibility.String())
}

func TestCompatibilityGiven(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifestCompatibility)

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	compatibility, err := app.GetShopwareVersionConstraint()
	assert.NoError(t, err)

	assert.Equal(t, "~6.5.0", compatibility.String())
}

func TestAppWithPHPFiles(t *testing.T) {
	appPath := testhelper.AppDir(t, testAppManifest)

	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))
	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "test.php"), []byte("<?php echo 'Hello World';"), 0o644))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Contains(t, check.Results[0].Message, "Found unexpected PHP file")
}

func TestAppWithTwigFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping test on windows")
	}

	appPath := testhelper.AppDir(t, testAppManifest)

	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), 0o755))
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/views/"), 0o755))

	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))
	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "test.twig"), []byte("<?php echo 'Hello World';"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "Resources/views/test.twig"), []byte("<?php echo 'Hello World';"), 0o644))

	app, err := newApp(t.Context(), appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Contains(t, check.Results[0].Message, "Twig files should be at")
}
