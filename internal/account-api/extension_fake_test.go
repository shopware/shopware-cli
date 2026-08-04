package account_api

import (
	"context"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

// fakeExtension is a minimal Extension double for account-api store tests.
type fakeExtension struct {
	name       string
	path       string
	extVersion *version.Version
	constraint *version.Constraints
	changelog  *extension.ExtensionChangelog
	metadata   *extension.ExtensionMetadata
	config     *extension.Config
	extType    string
	updated    *extension.ExtensionMetadata
}

func (f *fakeExtension) GetName() (string, error) {
	if f.name != "" {
		return f.name, nil
	}
	return "TestPlugin", nil
}

func (f *fakeExtension) GetComposerName() (string, error) { return "acme/test", nil }
func (f *fakeExtension) GetResourcesDir() string          { return "src/Resources" }
func (f *fakeExtension) GetResourcesDirs() []string       { return []string{"src/Resources"} }
func (f *fakeExtension) GetIconPath() string              { return "" }
func (f *fakeExtension) GetRootDir() string               { return "src" }
func (f *fakeExtension) GetSourceDirs() []string          { return []string{"src"} }

func (f *fakeExtension) GetVersion() (*version.Version, error) {
	if f.extVersion != nil {
		return f.extVersion, nil
	}
	return version.NewVersion("1.0.0")
}

func (f *fakeExtension) GetLicense() (string, error) { return "MIT", nil }

func (f *fakeExtension) GetShopwareVersionConstraint() (*version.Constraints, error) {
	if f.constraint != nil {
		return f.constraint, nil
	}
	c, err := version.NewConstraint(">=6.5.0")
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (f *fakeExtension) GetType() string {
	if f.extType != "" {
		return f.extType
	}
	return extension.TypePlatformPlugin
}

func (f *fakeExtension) GetPath() string { return f.path }

func (f *fakeExtension) GetChangelog() (*extension.ExtensionChangelog, error) {
	if f.changelog != nil {
		return f.changelog, nil
	}
	return &extension.ExtensionChangelog{German: "de", English: "en"}, nil
}

func (f *fakeExtension) GetMetaData() *extension.ExtensionMetadata {
	if f.metadata != nil {
		return f.metadata
	}
	return &extension.ExtensionMetadata{
		Label:       extension.ExtensionTranslated{German: "DE Label", English: "EN Label"},
		Description: extension.ExtensionTranslated{German: "DE Desc", English: "EN Desc"},
	}
}

func (f *fakeExtension) UpdateMetaData(m *extension.ExtensionMetadata) error {
	f.updated = m
	return nil
}

func (f *fakeExtension) GetExtensionConfig() *extension.Config {
	if f.config != nil {
		return f.config
	}
	return &extension.Config{FileName: ".shopware-extension.yml"}
}

func (f *fakeExtension) Validate(context.Context, validation.Check) {}

var _ extension.Extension = (*fakeExtension)(nil)
