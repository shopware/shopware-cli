package extension

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
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

func TestBuildModifierRewritesUrlsWithoutDroppingUnknownManifestXML(t *testing.T) {
	app := &App{}
	tmpDir := t.TempDir()
	manifest := strings.TrimSpace(`<?xml version="1.0" encoding="UTF-8"?>
<manifest xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xsi:noNamespaceSchemaLocation="https://example.com/schema.xsd">
    <meta>
        <version>1.0.0</version>
        <future-meta>keep</future-meta>
    </meta>
    <setup>
        <registrationUrl>http://localhost/register</registrationUrl>
    </setup>
    <admin>
        <base-app-url>http://localhost/admin</base-app-url>
        <action-button action="open" entity="product" view="detail" url="http://localhost/action">
            <label>Open</label>
            <future-action-button>keep</future-action-button>
        </action-button>
    </admin>
    <payments>
        <payment-method>
            <identifier>pay</identifier>
            <pay-url>http://localhost/pay</pay-url>
            <finalize-url>http://localhost/finalize</finalize-url>
            <unknown-payment-child>keep</unknown-payment-child>
        </payment-method>
    </payments>
    <tax>
        <tax-provider>
            <identifier>tax</identifier>
            <process-url>http://localhost/tax</process-url>
        </tax-provider>
    </tax>
    <webhooks>
        <webhook name="hook" url="http://localhost/webhook" event="checkout"/>
    </webhooks>
    <future-root>keep</future-root>
</manifest>`)

	assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "manifest.xml"), []byte(manifest), 0644))

	assert.NoError(t, BuildModifier(app, tmpDir, BuildModifierConfig{AppBackendUrl: "https://example.com"}))

	bytes, err := os.ReadFile(filepath.Join(tmpDir, "manifest.xml"))
	assert.NoError(t, err)
	output := string(bytes)

	assert.Contains(t, output, `xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`)
	assert.Contains(t, output, `xsi:noNamespaceSchemaLocation="https://example.com/schema.xsd"`)
	assert.Contains(t, output, "<future-meta>keep</future-meta>")
	assert.Contains(t, output, "<future-action-button>keep</future-action-button>")
	assert.Contains(t, output, "<unknown-payment-child>keep</unknown-payment-child>")
	assert.Contains(t, output, "<future-root>keep</future-root>")
	assert.Contains(t, output, "<registrationUrl>https://example.com/register</registrationUrl>")
	assert.Contains(t, output, "<base-app-url>https://example.com/admin</base-app-url>")
	assert.Contains(t, output, `url="https://example.com/action"`)
	assert.Contains(t, output, "<pay-url>https://example.com/pay</pay-url>")
	assert.Contains(t, output, "<finalize-url>https://example.com/finalize</finalize-url>")
	assert.Contains(t, output, "<process-url>https://example.com/tax</process-url>")
	assert.Contains(t, output, `url="https://example.com/webhook"`)
	assert.NotContains(t, output, "_xmlns")
}
