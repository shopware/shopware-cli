package upgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestRewriteComposerJSON(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Name: "shopware/production",
		Require: map[string]string{
			"shopware/administration": "6.6.10.3",
			"shopware/core":           "6.6.10.3",
			"shopware/storefront":     "6.6.10.3",
			"swag/demo":               "^2.0",
			"symfony/flex":            "~2",
		},
	}.String())
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testComposerLock)

	changes, err := newTestUpgrader(t, dir).RewriteComposerJSON("6.7.11.0", map[string]string{"swag/demo": "2.1.3"})
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{
		"shopware/core: 6.6.10.3 -> 6.7.11.0",
		"shopware/administration: 6.6.10.3 -> 6.7.11.0",
		"shopware/storefront: 6.6.10.3 -> 6.7.11.0",
		"swag/demo: ^2.0 -> 2.1.3",
		"shopware/deployment-helper: added",
	}, changes)

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	var parsed struct {
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(content, &parsed))

	assert.Equal(t, "6.7.11.0", parsed.Require["shopware/core"])
	assert.Equal(t, "6.7.11.0", parsed.Require["shopware/administration"])
	assert.Equal(t, "6.7.11.0", parsed.Require["shopware/storefront"])
	assert.Equal(t, "*", parsed.Require["shopware/deployment-helper"])
	assert.Equal(t, "~2", parsed.Require["symfony/flex"], "unrelated packages stay untouched")
	assert.Equal(t, "2.1.3", parsed.Require["swag/demo"], "extensions are pinned to the resolved release")
	assert.NotContains(t, parsed.Require, "shopware/elasticsearch", "absent platform packages are not added")
}

func TestRewriteComposerJSONKeepsPathRepositoryConstraints(t *testing.T) {
	dir := setupPathPluginProject(t)
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Name: "shopware/production",
		Require: map[string]string{
			"shopware/core":              "6.7.3.0",
			"shopware/deployment-helper": "*",
			"acme/custom-plugin":         "1.0.0",
			"swag/demo":                  "^2.0",
		},
	}.String())
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testhelper.ComposerLock(
		testhelper.LockPackage{Name: "shopware/core", Version: "v6.7.3.0"},
		testhelper.LockPackage{Name: "swag/demo", Version: "2.0.0", Type: "shopware-platform-plugin"},
		testhelper.LockPackage{
			Name: "acme/custom-plugin", Version: "1.0.0", Type: "shopware-platform-plugin",
			Require: map[string]string{"shopware/core": "~6.7.0"},
			Dist:    map[string]string{"type": "path", "url": "custom/static-plugins/MyCustomPlugin"},
		},
	))

	changes, err := newTestUpgrader(t, dir).RewriteComposerJSON("6.7.11.0", map[string]string{"swag/demo": "2.1.3"})
	require.NoError(t, err)
	assert.Contains(t, changes, "shopware/core: 6.7.3.0 -> 6.7.11.0")
	assert.Contains(t, changes, "swag/demo: ^2.0 -> 2.1.3")
	for _, change := range changes {
		assert.NotContains(t, change, "acme/custom-plugin")
	}

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	var parsed struct {
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.Equal(t, "1.0.0", parsed.Require["acme/custom-plugin"])
	assert.Equal(t, "2.1.3", parsed.Require["swag/demo"])
}

func TestRenderUpgradeManifestLeavesProjectUntouched(t *testing.T) {
	dir := t.TempDir()
	original := `{
		"name": "shopware/production",
		"require": {"shopware/core": "6.6.10.3", "shopware/deployment-helper": "*"}
	}`
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), original)
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testhelper.ComposerLock())

	manifest, err := newTestUpgrader(t, dir).renderUpgradeManifest("6.7.11.0")
	require.NoError(t, err)

	var parsed struct {
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(manifest, &parsed))
	assert.Equal(t, "6.7.11.0", parsed.Require["shopware/core"])

	onDisk, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, original, string(onDisk), "rendering the manifest must not modify composer.json")
}

func TestRewriteComposerJSONWithoutResolvedVersions(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Name:    "shopware/production",
		Require: map[string]string{"shopware/core": "6.6.10.3", "shopware/deployment-helper": "*", "swag/demo": "^2.0"},
	}.String())
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testComposerLock)

	changes, err := newTestUpgrader(t, dir).RewriteComposerJSON("6.7.11.0", nil)
	require.NoError(t, err)
	assert.Contains(t, changes, "swag/demo: ^2.0 -> *", "without a resolved version the constraint falls back to *")
}

func TestRewriteComposerJSONDoesNotMoveRequireDevPackages(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Name:       "shopware/production",
		RequireDev: map[string]string{"shopware/core": "6.6.10.3", "swag/demo": "^2.0"},
	}.String())
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testComposerLock)

	changes, err := newTestUpgrader(t, dir).RewriteComposerJSON("6.7.11.0", map[string]string{"swag/demo": "2.1.3"})
	require.NoError(t, err)
	assert.Equal(t, []string{"shopware/deployment-helper: added"}, changes)

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	var parsed struct {
		Require    map[string]string `json:"require"`
		RequireDev map[string]string `json:"require-dev"`
	}
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.Equal(t, "6.6.10.3", parsed.RequireDev["shopware/core"])
	assert.Equal(t, "^2.0", parsed.RequireDev["swag/demo"])
	assert.NotContains(t, parsed.Require, "shopware/core")
	assert.NotContains(t, parsed.Require, "swag/demo")
	assert.Equal(t, "*", parsed.Require["shopware/deployment-helper"])
}

func TestSetComposerName(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Require: map[string]string{"shopware/core": "6.6.10.3"},
	}.String())

	require.NoError(t, newTestUpgrader(t, dir).SetComposerName("acme/my-shop"))

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	var parsed struct {
		Name    string            `json:"name"`
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(content, &parsed))
	assert.Equal(t, "acme/my-shop", parsed.Name)
	assert.Equal(t, "6.6.10.3", parsed.Require["shopware/core"], "existing fields stay untouched")
}

func TestSetComposerNameRejectsInvalidNames(t *testing.T) {
	dir := t.TempDir()
	original := testhelper.ComposerJSON{
		Require: map[string]string{"shopware/core": "6.6.10.3"},
	}.String()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), original)

	for _, name := range []string{"", "no-slash", "Acme/Shop", "acme/my shop"} {
		assert.Error(t, newTestUpgrader(t, dir).SetComposerName(name), name)
	}

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, original, string(content), "an invalid name must not modify composer.json")
}

func TestSetComposerNameWithoutComposerJSON(t *testing.T) {
	err := newTestUpgrader(t, t.TempDir()).SetComposerName("acme/my-shop")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read composer.json")
}

func TestSuggestComposerName(t *testing.T) {
	vendor := defaultComposerVendor()
	assert.NoError(t, ValidateComposerName(vendor+"/production"), "vendor must be valid, got %q", vendor)

	tests := []struct {
		root string
		want string
	}{
		{root: "/srv/shops/acme-shop", want: "acme-shop"},
		{root: "/srv/shops/Acme Shop", want: "acme-shop"},
		{root: "/srv/shops/My_Shop 2", want: "my-shop-2"},
		{root: "/srv/shops/--weird--", want: "weird"},
		{root: "/srv/shops/1337", want: "1337"},
		{root: ".", want: "production"},
		{root: "/", want: "production"},
	}
	for _, test := range tests {
		got := SuggestComposerName(test.root)
		assert.Equal(t, vendor+"/"+test.want, got, test.root)
		assert.NoError(t, ValidateComposerName(got), test.root)
	}
}

func TestSanitizeComposerPart(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "shyim", want: "shyim"},
		{in: "Shyim Doe", want: "shyim-doe"},
		{in: "My_Shop 2", want: "my-shop-2"},
		{in: "--weird--", want: "weird"},
		{in: "", want: ""},
		{in: "---", want: ""},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, sanitizeComposerPart(test.in), "%q", test.in)
	}
}

func TestLockNameFor(t *testing.T) {
	assert.Equal(t, "composer.lock", lockNameFor("composer.json"))
	assert.Equal(t, ".shopware-cli-upgrade-composer.lock", lockNameFor(upgradeManifestName))
}

func TestRewriteComposerJSONLeavesAuditConfigUntouched(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), testhelper.ComposerJSON{
		Name:    "shopware/production",
		Require: map[string]string{"shopware/core": "6.6.10.3", "shopware/deployment-helper": "*"},
	}.String())
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testComposerLock)

	u := newTestUpgrader(t, dir)
	u.DisableAuditBlock()

	changes, err := u.RewriteComposerJSON("6.7.11.0", nil)
	require.NoError(t, err)
	for _, change := range changes {
		assert.NotContains(t, change, "audit")
	}

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), "block-insecure",
		"the audit opt-out is passed as COMPOSER_NO_SECURITY_BLOCKING, never persisted")

	manifest, err := u.renderUpgradeManifest("6.7.11.0")
	require.NoError(t, err)
	assert.NotContains(t, string(manifest), "block-insecure")
}
