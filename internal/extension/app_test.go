package extension

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testAppManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
		<license>MIT</license>
	</meta>
</manifest>`

const testAppManifestMissingLicense = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
	</meta>
</manifest>`

const testAppManifestMissingCopyright = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<author>Your Company Ltd.</author>
		<version>1.0.0</version>
		<license>MIT</license>
	</meta>
</manifest>`

const testAppManifestMissingAuthor = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
		<license>MIT</license>
	</meta>
</manifest>`

const testAppManifestCompatibility = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<compatibility>~6.5.0</compatibility>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
		<license>MIT</license>
	</meta>
</manifest>`

const testAppManifestIcon = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
		<license>MIT</license>
		<icon>app.png</icon>
	</meta>
</manifest>`

const testAppManifestSetup = `<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://raw.githubusercontent.com/shopware/shopware/trunk/src/Core/Framework/App/Manifest/Schema/manifest-2.0.xsd">
	<meta>
		<name>MyExampleApp</name>
		<label>Label</label>
		<label lang="de-DE">Name</label>
		<description>A description</description>
		<description lang="de-DE">Eine Beschreibung</description>
		<compatibility>~6.5.0</compatibility>
		<author>Your Company Ltd.</author>
		<copyright>(c) by Your Company Ltd.</copyright>
		<version>1.0.0</version>
		<license>MIT</license>
	</meta>
	<setup>
		<secret>foo</secret>
	</setup>
</manifest>`

func TestIconNotExists(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifest), os.ModePerm))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	assert.Equal(t, "MyExampleApp", app.manifest.Meta.Name)
	assert.Equal(t, "", app.manifest.Meta.Icon)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The extension icon Resources/config/plugin.png does not exist", check.Results[0].Message)
}

func TestAppNoLicense(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifestMissingLicense), os.ModePerm))
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The element meta:license was not found in the manifest.xml", check.Results[0].Message)
}

func TestAppNoCopyright(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifestMissingCopyright), os.ModePerm))
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The element meta:copyright was not found in the manifest.xml", check.Results[0].Message)
}

func TestAppNoAuthor(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifestMissingAuthor), os.ModePerm))
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The element meta:author was not found in the manifest.xml", check.Results[0].Message)
}

func TestAppHasSecret(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifestSetup), os.ModePerm))
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The xml element setup:secret is only for local development, please remove it. You can find your generated app secret on your extension detail page in the master data section. For more information see https://docs.shopware.com/en/shopware-platform-dev-en/app-system-guide/setup#authorisation", check.Results[0].Message)
}

func TestIconExistsDefaultsPath(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifest), os.ModePerm))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	assert.Equal(t, "MyExampleApp", app.manifest.Meta.Name)
	assert.Equal(t, "", app.manifest.Meta.Icon)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 0, len(check.Results))
}

func TestIconExistsDifferentPath(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifestIcon), os.ModePerm))
	assert.NoError(t, createTestImageWithSize(filepath.Join(appPath, "app.png"), 120, 120))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	assert.Equal(t, "MyExampleApp", app.manifest.Meta.Name)
	assert.Equal(t, "app.png", app.manifest.Meta.Icon)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 0, len(check.Results))
}

func TestNoCompatibilityGiven(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifest), os.ModePerm))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	compatibility, err := app.GetShopwareVersionConstraint()
	assert.NoError(t, err)

	assert.Equal(t, "~6.4", compatibility.String())
}

func TestCompatibilityGiven(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifestCompatibility), os.ModePerm))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	compatibility, err := app.GetShopwareVersionConstraint()
	assert.NoError(t, err)

	assert.Equal(t, "~6.5.0", compatibility.String())
}

func TestAppWithPHPFiles(t *testing.T) {
	appPath := t.TempDir()

	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifest), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))
	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "test.php"), []byte("<?php echo 'Hello World';"), os.ModePerm))

	app, err := newApp(appPath)

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

	appPath := t.TempDir()

	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/config"), os.ModePerm))
	assert.NoError(t, os.MkdirAll(filepath.Join(appPath, "Resources/views/"), os.ModePerm))

	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "manifest.xml"), []byte(testAppManifest), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(appPath, "Resources/config/plugin.png")))
	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "test.twig"), []byte("<?php echo 'Hello World';"), os.ModePerm))
	assert.NoError(t, os.WriteFile(filepath.Join(appPath, "Resources/views/test.twig"), []byte("<?php echo 'Hello World';"), os.ModePerm))

	app, err := newApp(appPath)

	assert.NoError(t, err)

	check := &testCheck{}
	app.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Contains(t, check.Results[0].Message, "Twig files should be at")
}
