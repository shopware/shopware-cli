package extension

import (
	"encoding/xml"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
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
