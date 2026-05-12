package extension

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/validation"
)

func setupMockPHPVersionServer(t *testing.T) {
	t.Helper()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"6.4.0.0": "7.4", "6.5.0.0": "8.1", "6.6.0.0": "8.2"}`))
	}))
	t.Cleanup(server.Close)

	original := phpVersionURL
	phpVersionURL = server.URL
	t.Cleanup(func() {
		phpVersionURL = original
	})

	originalValidate := validatePHPFilesFn
	validatePHPFilesFn = func(_ context.Context, _ Extension, _ validation.Check) {}
	t.Cleanup(func() {
		validatePHPFilesFn = originalValidate
	})
}

func getTestPlugin(tempDir string) PlatformPlugin {
	return PlatformPlugin{
		path: tempDir,
		config: &Config{
			Store: ConfigStore{
				Availabilities: &[]string{"German"},
			},
		},
		Composer: PlatformComposerJson{
			Name:        "frosh/frosh-tools",
			Description: "Frosh Tools",
			License:     "mit",
			Version:     "1.0.0",
			Require:     map[string]string{"shopware/core": "6.4.0.0"},
			Autoload: struct {
				Psr0 map[string]string `json:"psr-0"`
				Psr4 map[string]string `json:"psr-4"`
			}{Psr0: map[string]string{"FroshTools\\": "src/"}, Psr4: map[string]string{"FroshTools\\": "src/"}},
			Authors: []struct {
				Name     string `json:"name"`
				Homepage string `json:"homepage"`
			}{{Name: "Frosh", Homepage: "https://frosh.io"}},
			Type: "shopware-platform-plugin",
			Extra: platformComposerJsonExtra{
				ShopwarePluginClass: "FroshTools\\FroshTools",
				Label: map[string]string{
					"en-GB": "Frosh Tools",
					"de-DE": "Frosh Tools",
				},
				Description: map[string]string{
					"en-GB": "Frosh Tools",
					"de-DE": "Frosh Tools",
				},
				ManufacturerLink: map[string]string{
					"en-GB": "Frosh Tools",
					"de-DE": "Frosh Tools",
				},
				SupportLink: map[string]string{
					"en-GB": "Frosh Tools",
					"de-DE": "Frosh Tools",
				},
			},
		},
	}
}

func TestPluginIconNotExists(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)

	check := &testCheck{}

	plugin.Validate(getTestContext(), check)

	assert.Equal(t, 1, len(check.Results))
	assert.Equal(t, "The extension icon Resources/config/plugin.png does not exist", check.Results[0].Message)
}

func TestPluginIconExists(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)

	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "Resources", "config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(dir, "src", "Resources", "config", "plugin.png")))

	check := &testCheck{}

	plugin.Validate(getTestContext(), check)

	assert.Equal(t, 0, len(check.Results))
}

func TestPluginIconDifferntPathExists(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)
	plugin.Composer.Extra.PluginIcon = "plugin.png"

	assert.NoError(t, createTestImage(filepath.Join(dir, "plugin.png")))

	check := &testCheck{}

	plugin.Validate(getTestContext(), check)

	assert.Equal(t, 0, len(check.Results))
}

func TestPluginIconIsTooBig(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)

	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "Resources", "config"), os.ModePerm))
	assert.NoError(t, createTestImageWithSize(filepath.Join(dir, "src", "Resources", "config", "plugin.png"), 1000, 1000))

	check := &testCheck{}

	plugin.Validate(getTestContext(), check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "The extension icon Resources/config/plugin.png dimensions (1000x1000) are larger than maximum 256x256 pixels with max file size 30kb and 72dpi", check.Results[0].Message)
}

func TestPluginGermanDescriptionMissing(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)
	plugin.Composer.Extra.Description = map[string]string{
		"en-GB": "Frosh Tools",
	}

	check := &testCheck{}
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "Resources", "config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(dir, "src", "Resources", "config", "plugin.png")))

	plugin.Validate(getTestContext(), check)

	assert.Len(t, check.Results, 1)
	assert.Equal(t, "extra.description for language de-DE is required", check.Results[0].Message)
}

func TestPluginGermanDescriptionMissingOnlyEnglishMarket(t *testing.T) {
	setupMockPHPVersionServer(t)
	dir := t.TempDir()

	plugin := getTestPlugin(dir)
	plugin.Composer.Extra.Description = map[string]string{
		"en-GB": "Frosh Tools",
	}
	plugin.config.Store.Availabilities = &[]string{"International"}
	assert.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "Resources", "config"), os.ModePerm))
	assert.NoError(t, createTestImage(filepath.Join(dir, "src", "Resources", "config", "plugin.png")))

	check := &testCheck{}

	plugin.Validate(getTestContext(), check)

	assert.Len(t, check.Results, 0)
}

func TestLowestPhpVersionForConstraint(t *testing.T) {
	cases := []struct {
		constraint string
		expected   string
	}{
		{">=8.4", "8.4"},
		{"^8.2", "8.2"},
		{"~8.1.0", "8.1"},
		{">=8.0 <8.3", "8.0"},
		{"8.3.*", "8.3"},
		{"^7.4 || ^8.0", "7.4"},
	}

	for _, tc := range cases {
		t.Run(tc.constraint, func(t *testing.T) {
			got, err := lowestPhpVersionForConstraint(tc.constraint)
			assert.NoError(t, err)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestLowestPhpVersionForConstraintInvalid(t *testing.T) {
	_, err := lowestPhpVersionForConstraint("not-a-constraint")
	assert.Error(t, err)
}

func TestNormalizePhpVersion(t *testing.T) {
	cases := map[string]string{
		"8.4":      "8.4",
		"8.4.1":    "8.4",
		" 8.2 ":    "8.2",
		"8":        "8",
		"7.4.33-1": "7.4",
	}

	for input, expected := range cases {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, expected, normalizePhpVersion(input))
		})
	}
}

func TestResolvePhpVersionOverride(t *testing.T) {
	setupMockPHPVersionServer(t)
	c, err := version.NewConstraint(">= 6.5")
	assert.NoError(t, err)

	got, err := ResolvePhpVersion(t.Context(), "8.4", "^8.2", &c)
	assert.NoError(t, err)
	assert.Equal(t, "8.4", got)
}

func TestResolvePhpVersionFromComposer(t *testing.T) {
	setupMockPHPVersionServer(t)
	c, err := version.NewConstraint(">= 6.5")
	assert.NoError(t, err)

	got, err := ResolvePhpVersion(t.Context(), "", ">=8.4", &c)
	assert.NoError(t, err)
	assert.Equal(t, "8.4", got)
}

func TestResolvePhpVersionFallsBackToStaticMapping(t *testing.T) {
	setupMockPHPVersionServer(t)
	c, err := version.NewConstraint("6.5.0.0")
	assert.NoError(t, err)

	got, err := ResolvePhpVersion(t.Context(), "", "", &c)
	assert.NoError(t, err)
	assert.Equal(t, "8.1", got)
}

func TestResolvePhpVersionFallsBackOnInvalidComposerConstraint(t *testing.T) {
	setupMockPHPVersionServer(t)
	c, err := version.NewConstraint("6.5.0.0")
	assert.NoError(t, err)

	got, err := ResolvePhpVersion(t.Context(), "", "not-a-constraint", &c)
	assert.NoError(t, err)
	assert.Equal(t, "8.1", got)
}

func TestGetComposerRequirePhp(t *testing.T) {
	plugin := &PlatformPlugin{
		Composer: PlatformComposerJson{
			Require: map[string]string{
				"php":           "^8.2",
				"shopware/core": "^6.6",
			},
		},
	}
	assert.Equal(t, "^8.2", GetComposerRequirePhp(plugin))

	pluginWithoutPhp := &PlatformPlugin{
		Composer: PlatformComposerJson{
			Require: map[string]string{
				"shopware/core": "^6.6",
			},
		},
	}
	assert.Equal(t, "", GetComposerRequirePhp(pluginWithoutPhp))

	bundle := &ShopwareBundle{
		Composer: shopwareBundleComposerJson{
			Require: map[string]string{
				"php":           "^8.3",
				"shopware/core": "^6.6",
			},
		},
	}
	assert.Equal(t, "^8.3", GetComposerRequirePhp(bundle))

	app := &App{}
	assert.Equal(t, "", GetComposerRequirePhp(app))
}
