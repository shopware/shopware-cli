package extension

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

const exampleManifest = `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
	<meta>
		<version>1.0.0</version>
	</meta>
    <setup>
	  <registrationUrl>http://localhost/foo</registrationUrl>
    </setup>
</manifest>`

func TestSetVersionApp(t *testing.T) {
	app := &App{}

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(exampleManifest), 0644))

	assert.NoError(t, BuildModifier(app, tmpDir, BuildModifierConfig{Version: "5.0.0"}))

	bytes, err := os.ReadFile(filepath.Join(tmpDir, "manifest.xml"))

	assert.NoError(t, err)

	var manifest Manifest

	assert.NoError(t, xml.Unmarshal(bytes, &manifest))

	assert.Equal(t, "5.0.0", manifest.Meta.Version)
}

func TestSetRegistration(t *testing.T) {
	app := &App{}

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(exampleManifest), 0644))

	assert.NoError(t, BuildModifier(app, tmpDir, BuildModifierConfig{AppBackendUrl: "https://foo.com"}))

	bytes, err := os.ReadFile(filepath.Join(tmpDir, "manifest.xml"))

	assert.NoError(t, err)

	var manifest Manifest

	assert.NoError(t, xml.Unmarshal(bytes, &manifest))

	assert.Equal(t, "https://foo.com/foo", manifest.Setup.RegistrationUrl)
}

func TestSetRegistrationSecret(t *testing.T) {
	app := &App{}

	tmpDir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(exampleManifest), 0644))

	assert.NoError(t, BuildModifier(app, tmpDir, BuildModifierConfig{AppBackendSecret: "secret"}))

	bytes, err := os.ReadFile(filepath.Join(tmpDir, "manifest.xml"))

	assert.NoError(t, err)

	var manifest Manifest

	assert.NoError(t, xml.Unmarshal(bytes, &manifest))

	assert.Equal(t, "http://localhost/foo", manifest.Setup.RegistrationUrl)
	assert.Equal(t, "secret", manifest.Setup.Secret)
}
