package extension

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestRead(t *testing.T) {
	bytes, err := os.ReadFile("_fixtures/istorier.xml")

	assert.NoError(t, err)

	manifest := Manifest{}

	assert.NoError(t, xml.Unmarshal(bytes, &manifest))

	assert.Equal(t, "InstoImmersiveElements", manifest.Meta.Name)
	assert.Equal(t, "Immersive Elements", manifest.Meta.Label[0].Value)
	assert.Equal(t, "Transform your online store into an unforgettable brand experience. As an incredibly cost-effective alternative to external resources, the app is engineered to boost conversions.", manifest.Meta.Description[0].Value)
	assert.Equal(t, "Instorier AS", manifest.Meta.Author)
	assert.Equal(t, "(c) by Instorier AS", manifest.Meta.Copyright)
	assert.Equal(t, "1.1.0", manifest.Meta.Version)
	assert.Equal(t, "Resources/config/plugin.png", manifest.Meta.Icon)
	assert.Equal(t, "Proprietary", manifest.Meta.License)

	assert.Equal(t, "https://instorier.apps.shopware.io/app/lifecycle/register", manifest.Setup.RegistrationUrl)
	assert.Equal(t, "", manifest.Setup.Secret)

	assert.Equal(t, "https://instorier.apps.shopware.io/iframe", manifest.Admin.BaseAppUrl)

	assert.Len(t, manifest.Permissions.Read, 57)
	assert.Len(t, manifest.Permissions.Create, 4)
	assert.Len(t, manifest.Permissions.Update, 2)
	assert.Len(t, manifest.Permissions.Delete, 2)
}

func TestManifestRoundTripPreservesUnknownElements(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
    <future-element>
        <some-nested>value</some-nested>
    </future-element>
    <permissions>
        <read>product</read>
    </permissions>
    <another-new-tag>hello</another-new-tag>
</manifest>`

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))

	assert.Equal(t, "TestApp", manifest.Meta.Name)
	assert.Equal(t, "1.0.0", manifest.Meta.Version)
	assert.NotNil(t, manifest.Permissions)
	assert.Len(t, manifest.Permissions.Read, 1)

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "<future-element>")
	assert.Contains(t, output, "<some-nested>value</some-nested>")
	assert.Contains(t, output, "<another-new-tag>hello</another-new-tag>")
	assert.Contains(t, output, "<name>TestApp</name>")
	assert.Contains(t, output, "<read>product</read>")
}

func TestManifestRoundTripWithModification(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
    <setup>
        <registrationUrl>https://example.com</registrationUrl>
        <secret>mysecret</secret>
    </setup>
    <unknown-future-element>preserve me</unknown-future-element>
</manifest>`

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))

	manifest.Setup.Secret = ""

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "<unknown-future-element>preserve me</unknown-future-element>")
	assert.Contains(t, output, "<registrationUrl>https://example.com</registrationUrl>")
	assert.NotContains(t, output, "<secret>mysecret</secret>")
}

func TestManifestValidatesPermissionsAttribute(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<manifest validates-permissions="true">
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
</manifest>`

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))
	require.NotNil(t, manifest.ValidatesPermissions)
	assert.True(t, *manifest.ValidatesPermissions)

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)
	assert.Contains(t, string(out), `validates-permissions="true"`)
}

func TestManifestRequirements(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
    <requirements>
        <min-shopware-version>6.6.0</min-shopware-version>
    </requirements>
</manifest>`

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))
	require.NotNil(t, manifest.Requirements)

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "<requirements>")
	assert.Contains(t, output, "<min-shopware-version>6.6.0</min-shopware-version>")
	assert.Contains(t, output, "</requirements>")
}

func TestManifestEmptyRoundTrip(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
</manifest>`

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "<name>TestApp</name>")
	assert.NotContains(t, output, "<setup>")
	assert.NotContains(t, output, "<admin>")
}

func TestManifestUnmarshalDoesNotCaptureKnownElements(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<manifest>
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
    <permissions>
        <read>product</read>
    </permissions>
</manifest>`

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))

	assert.NotNil(t, manifest.Permissions)
	assert.Len(t, manifest.Permissions.Read, 1)

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "<name>TestApp</name>")
	assert.Contains(t, output, "<read>product</read>")
	assert.NotContains(t, output, "future-element")
	assert.NotContains(t, output, "unknown")
}

func TestManifestWhitespacePreservedInRemaining(t *testing.T) {
	input := strings.TrimSpace(`<?xml version="1.0" encoding="UTF-8"?>
<manifest>
    <meta>
        <name>TestApp</name>
        <label>Test</label>
        <version>1.0.0</version>
        <license>MIT</license>
    </meta>
    <custom-thing>
        <nested>
            <deep>value</deep>
        </nested>
    </custom-thing>
</manifest>`)

	var manifest Manifest
	require.NoError(t, xml.Unmarshal([]byte(input), &manifest))

	out, err := xml.MarshalIndent(&manifest, "", "  ")
	require.NoError(t, err)

	output := string(out)
	assert.Contains(t, output, "<custom-thing>")
	assert.Contains(t, output, "<nested>")
	assert.Contains(t, output, "<deep>value</deep>")
	assert.Contains(t, output, "</nested>")
	assert.Contains(t, output, "</custom-thing>")
}
