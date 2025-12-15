package extension

import (
	"context"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/validation"
)

type mockExtension struct {
	iconPath   string
	path       string
	name       string
	extVersion *version.Version
	config     *Config
	rootDir    string
}

func (m *mockExtension) GetName() (string, error) {
	if m.name != "" {
		return m.name, nil
	}
	return "test", nil
}

func (m *mockExtension) GetComposerName() (string, error) {
	return "test/test", nil
}

func (m *mockExtension) GetResourcesDir() string {
	return "src/Resources"
}

func (m *mockExtension) GetResourcesDirs() []string {
	return []string{"src/Resources"}
}

func (m *mockExtension) GetIconPath() string {
	return m.iconPath
}

func (m *mockExtension) GetRootDir() string {
	if m.rootDir != "" {
		return m.rootDir
	}
	return "src"
}

func (m *mockExtension) GetSourceDirs() []string {
	return []string{"src"}
}

func (m *mockExtension) GetVersion() (*version.Version, error) {
	if m.extVersion != nil {
		return m.extVersion, nil
	}
	return version.NewVersion("1.0.0")
}

func (m *mockExtension) GetLicense() (string, error) {
	return "MIT", nil
}

func (m *mockExtension) GetShopwareVersionConstraint() (*version.Constraints, error) {
	c, err := version.NewConstraint(">= 6.5")
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (m *mockExtension) GetType() string {
	return "test"
}

func (m *mockExtension) GetPath() string {
	return m.path
}

func (m *mockExtension) GetChangelog() (*ExtensionChangelog, error) {
	return &ExtensionChangelog{}, nil
}

func (m *mockExtension) GetMetaData() *extensionMetadata {
	return nil
}

func (m *mockExtension) GetExtensionConfig() *Config {
	return m.config
}

func (m *mockExtension) Validate(ctx context.Context, check validation.Check) {
}

func TestMockExtension(t *testing.T) {
	ext := &mockExtension{}

	name, err := ext.GetName()
	assert.NoError(t, err)
	assert.Equal(t, "test", name)

	composerName, err := ext.GetComposerName()
	assert.NoError(t, err)
	assert.Equal(t, "test/test", composerName)

	resourcesDir := ext.GetResourcesDir()
	assert.Equal(t, "src/Resources", resourcesDir)

	iconPath := ext.GetIconPath()
	assert.Equal(t, "", iconPath)

	rootDir := ext.GetRootDir()
	assert.Equal(t, "src", rootDir)

	sourceDirs := ext.GetSourceDirs()
	assert.Equal(t, []string{"src"}, sourceDirs)

	extVersion, err := ext.GetVersion()
	assert.NoError(t, err)
	assert.Equal(t, "1.0.0", extVersion.String())

	license, err := ext.GetLicense()
	assert.NoError(t, err)
	assert.Equal(t, "MIT", license)

	constraint, err := ext.GetShopwareVersionConstraint()
	assert.NoError(t, err)
	v650, _ := version.NewVersion("6.5.0")
	assert.True(t, constraint.Check(v650))

	extType := ext.GetType()
	assert.Equal(t, "test", extType)

	changelog, err := ext.GetChangelog()
	assert.NoError(t, err)
	assert.NotNil(t, changelog)

	metaData := ext.GetMetaData()
	assert.Nil(t, metaData)

	config := ext.GetExtensionConfig()
	assert.Nil(t, config)
}
