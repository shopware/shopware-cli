package extension

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestCreateBundleEmptyFolder(t *testing.T) {
	dir := t.TempDir()

	bundle, err := newShopwareBundle(t.Context(), dir)
	assert.Error(t, err)
	assert.Nil(t, bundle)
}

func TestCreateBundleInvalidComposerType(t *testing.T) {
	dir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
		Name: "shopware/invalid",
		Type: "invalid",
	})

	bundle, err := newShopwareBundle(t.Context(), dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "composer.json type is not shopware-bundle")
	assert.Nil(t, bundle)
}

func TestCreateBundleMissingName(t *testing.T) {
	dir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
		Name: "shopware/invalid",
		Type: "shopware-bundle",
	})

	bundle, err := newShopwareBundle(t.Context(), dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "composer.json does not contain shopware-bundle-name")
	assert.Nil(t, bundle)
}

func TestCreateBundle(t *testing.T) {
	dir := testhelper.ExtensionDir(t, testhelper.ComposerJSON{
		Name:    "shopware/invalid",
		Version: "1.0.0",
		Type:    "shopware-bundle",
		Extra:   map[string]any{"shopware-bundle-name": "TestBundle"},
		Psr4:    map[string]string{`TestBundle\`: "src/"},
	})

	bundle, err := newShopwareBundle(t.Context(), dir)
	assert.NoError(t, err)

	name, err := bundle.GetName()
	assert.NoError(t, err)

	assert.Equal(t, "TestBundle", name)
	assert.Equal(t, path.Join(dir, "src"), bundle.GetRootDir())
	assert.Equal(t, dir, bundle.GetPath())
	assert.Equal(t, path.Join(dir, "src", "Resources"), bundle.GetResourcesDir())
	assert.Equal(t, path.Join(dir, "src", "Resources"), bundle.GetResourcesDirs()[0])
	assert.Equal(t, TypeShopwareBundle, bundle.GetType())

	_, err = bundle.GetChangelog()
	// changelog is missing
	assert.Error(t, err)

	version, err := bundle.GetVersion()
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", version.String())

	// does nothing
	bundle.Validate(getTestContext(), &testCheck{})

	assert.Equal(t, "FALLBACK", bundle.GetMetaData().Description.German)
}
