package extension

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sbomTestComposerJSON = `{
  "name": "acme/sbom-test",
  "description": "SBOM test plugin",
  "type": "shopware-platform-plugin",
  "version": "1.2.3",
  "license": "proprietary",
  "authors": [{"name": "Acme GmbH", "homepage": "https://acme.example"}],
  "require": {
    "php": ">=8.2",
    "ext-json": "*",
    "shopware/core": ">=6.6.5 <6.8.0",
    "shopware/storefront": "~6.6.0",
    "acme/bundled-lib": "^2.1"
  },
  "autoload": {"psr-4": {"Acme\\SbomTest\\": "src/"}},
  "extra": {
    "shopware-plugin-class": "Acme\\SbomTest\\AcmeSbomTest",
    "label": {"de-DE": "SBOM Test", "en-GB": "SBOM Test"},
    "manufacturerLink": {"de-DE": "https://acme.example/de", "en-GB": "https://acme.example/en"}
  }
}`

const sbomTestInstalledJSON = `{
  "packages": [
    {
      "name": "acme/bundled-lib",
      "version": "2.1.4",
      "type": "library",
      "license": ["MIT"],
      "require": {"php": ">=8.1", "psr/log": "^3.0"},
      "dist": {"type": "zip", "url": "https://example.test/lib.zip", "shasum": "abc123"}
    },
    {
      "name": "psr/log",
      "version": "3.0.2",
      "type": "library",
      "license": ["MIT"]
    },
    {
      "name": "phpstan/phpstan",
      "version": "2.1.0",
      "type": "library",
      "license": ["MIT"]
    }
  ],
  "dev": true,
  "dev-package-names": ["phpstan/phpstan"]
}`

const sbomTestPackageLock = `{
  "name": "administration",
  "lockfileVersion": 3,
  "packages": {
    "": {"name": "administration", "dependencies": {"lodash": "^4.17.21", "@acme/ui": "1.0.0"}},
    "node_modules/lodash": {
      "version": "4.17.21",
      "resolved": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz",
      "integrity": "sha512-v2kDEe57lecTulaDIuNTPy3Ry4gLGJ6Z1O3vE1krgXZNrsQ+LFTGHVxVjcXPs17LhbZVGedAJv8XZ1tvj5FvSg==",
      "license": "MIT"
    },
    "node_modules/@acme/ui": {"version": "1.0.0", "license": "Apache-2.0"},
    "node_modules/@acme/ui/node_modules/lodash": {"version": "4.17.21", "license": "MIT"},
    "node_modules/eslint": {"version": "9.0.0", "dev": true, "license": "MIT"},
    "node_modules/fsevents": {"version": "2.3.3", "devOptional": true, "license": "MIT"}
  }
}`

func writeSBOMTestPlugin(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string]string{
		"composer.json":                                      sbomTestComposerJSON,
		"src/AcmeSbomTest.php":                               "<?php\n",
		"vendor/composer/installed.json":                     sbomTestInstalledJSON,
		"src/Resources/app/administration/package-lock.json": sbomTestPackageLock,
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return dir
}

func findSBOMComponent(t *testing.T, bom *ExtensionBOM, bomRef string) ExtensionComponent {
	t.Helper()

	for _, component := range bom.Components {
		if component.BOMRef == bomRef {
			return component
		}
	}

	t.Fatalf("component %s not found", bomRef)

	return ExtensionComponent{}
}

func TestGenerateSBOMForPlugin(t *testing.T) {
	dir := writeSBOMTestPlugin(t)

	ext, err := GetExtensionByFolder(t.Context(), dir)
	require.NoError(t, err)

	bom, err := GenerateSBOM(t.Context(), ext, dir, SBOMOptions{ToolVersion: "1.0.0"})
	require.NoError(t, err)

	root := bom.Metadata.Component
	require.NotNil(t, root)
	assert.Equal(t, "acme", root.Group)
	assert.Equal(t, "sbom-test", root.Name)
	assert.Equal(t, "1.2.3", root.Version)
	assert.Equal(t, "pkg:composer/acme/sbom-test@1.2.3", root.PURL)
	assert.Equal(t, "proprietary", root.Licenses[0].License.Name)

	require.NotNil(t, bom.Metadata.Manufacturer)
	assert.Equal(t, "Acme GmbH", bom.Metadata.Manufacturer.Name)
	assert.Equal(t, []string{"https://acme.example", "https://acme.example/en", "https://acme.example/de"}, bom.Metadata.Manufacturer.URL)

	bundled := findSBOMComponent(t, bom, "pkg:composer/acme/bundled-lib@2.1.4")
	assert.False(t, bundled.IsExternal)
	assert.Equal(t, "2.1.4", bundled.Version)

	dev := findSBOMComponent(t, bom, "pkg:composer/phpstan/phpstan@2.1.0")
	assert.Equal(t, []sbomProperty{{Name: "shopware-cli:composer:dev", Value: "true"}}, dev.Properties)

	core := findSBOMComponent(t, bom, "pkg:composer/shopware/core")
	assert.True(t, core.IsExternal)
	assert.Empty(t, core.Version)
	assert.Equal(t, "vers:composer/>=6.6.5|<6.8.0", core.VersionRange)
	assert.Equal(t, []sbomProperty{{Name: sbomPropertyComposerConstraint, Value: ">=6.6.5 <6.8.0"}}, core.Properties)

	storefront := findSBOMComponent(t, bom, "pkg:composer/shopware/storefront")
	assert.Equal(t, "vers:composer/>=6.6.0|<6.7.0", storefront.VersionRange)

	php := findSBOMComponent(t, bom, "platform:php")
	assert.Equal(t, "platform", php.Type)
	assert.True(t, php.IsExternal)
	assert.Equal(t, "vers:composer/>=8.2.0", php.VersionRange)

	lodash := findSBOMComponent(t, bom, "pkg:npm/lodash@4.17.21")
	assert.Equal(t, "MIT", lodash.Licenses[0].License.ID)
	require.Len(t, lodash.Hashes, 1)
	assert.Equal(t, "SHA-512", lodash.Hashes[0].Alg)

	scoped := findSBOMComponent(t, bom, "pkg:npm/%40acme/ui@1.0.0")
	assert.Equal(t, "@acme", scoped.Group)
	assert.Equal(t, "ui", scoped.Name)

	refs := make([]string, 0, len(bom.Components))
	for _, component := range bom.Components {
		refs = append(refs, component.BOMRef)
	}

	// Bundled composer packages are not listed again as external, platform
	// extensions are skipped, npm dev packages and duplicates are dropped.
	assert.ElementsMatch(t, []string{
		"pkg:composer/acme/bundled-lib@2.1.4",
		"pkg:composer/psr/log@3.0.2",
		"pkg:composer/phpstan/phpstan@2.1.0",
		"platform:php",
		"pkg:composer/shopware/core",
		"pkg:composer/shopware/storefront",
		"pkg:npm/%40acme/ui@1.0.0",
		"pkg:npm/lodash@4.17.21",
	}, refs)

	require.NotEmpty(t, bom.Dependencies)
	assert.Equal(t, "app:acme/sbom-test@1.2.3", bom.Dependencies[0].Ref)
	assert.ElementsMatch(t, refs, bom.Dependencies[0].DependsOn)
}

func TestGenerateSBOMWithoutBundledDependencies(t *testing.T) {
	dir := writeSBOMTestPlugin(t)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "vendor")))
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "src", "Resources")))

	ext, err := GetExtensionByFolder(t.Context(), dir)
	require.NoError(t, err)

	bom, err := GenerateSBOM(t.Context(), ext, dir, SBOMOptions{})
	require.NoError(t, err)

	for _, component := range bom.Components {
		assert.True(t, component.IsExternal, component.BOMRef)
	}

	// Without installed.json the composer requirement is only external.
	findSBOMComponent(t, bom, "pkg:composer/acme/bundled-lib")
}

func TestExtensionSBOMMarshalsCycloneDXFields(t *testing.T) {
	dir := writeSBOMTestPlugin(t)

	ext, err := GetExtensionByFolder(t.Context(), dir)
	require.NoError(t, err)

	output := filepath.Join(t.TempDir(), "out", "sbom.cdx.json")
	require.NoError(t, WriteSBOM(t.Context(), ext, dir, SBOMOptions{}, output))

	content, err := os.ReadFile(output)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(content, &doc))

	assert.Equal(t, "CycloneDX", doc["bomFormat"])
	assert.NotEmpty(t, doc["specVersion"])

	metadata := doc["metadata"].(map[string]any)
	assert.Equal(t, "Acme GmbH", metadata["manufacturer"].(map[string]any)["name"])
	assert.Equal(t, "pkg:composer/acme/sbom-test@1.2.3", metadata["component"].(map[string]any)["purl"])
	assert.NotNil(t, metadata["tools"])

	var external map[string]any
	for _, raw := range doc["components"].([]any) {
		component := raw.(map[string]any)
		if component["bom-ref"] == "pkg:composer/shopware/core" {
			external = component
		}
	}

	require.NotNil(t, external)
	assert.Equal(t, true, external["isExternal"])
	assert.Equal(t, "vers:composer/>=6.6.5|<6.8.0", external["versionRange"])
	assert.NotContains(t, external, "version")
}

func TestComposerConstraintToVers(t *testing.T) {
	cases := map[string]string{
		"*":                  "vers:composer/*",
		">=6.6.5 <6.8.0":     "vers:composer/>=6.6.5|<6.8.0",
		">=6.6.5, <6.8.0":    "vers:composer/>=6.6.5|<6.8.0",
		"<6.8.0 >=6.6.5":     "vers:composer/>=6.6.5|<6.8.0",
		"~6.6.0":             "vers:composer/>=6.6.0|<6.7.0",
		"~6.6":               "vers:composer/>=6.6.0|<7.0.0",
		"^2.1":               "vers:composer/>=2.1.0|<3.0.0",
		"^0.3.1":             "vers:composer/>=0.3.1|<0.4.0",
		"6.6.*":              "vers:composer/>=6.6.0|<6.7.0",
		"1.2.3":              "vers:composer/1.2.3",
		"v1.2.3":             "vers:composer/1.2.3",
		">=8.2":              "vers:composer/>=8.2.0",
		">= 8.3":             "vers:composer/>=8.3.0",
		">= 6.6.5, < 6.8.0":  "vers:composer/>=6.6.5|<6.8.0",
		"^6.6 || ^6.7":       "",
		"1.0 - 2.0":          "",
		"dev-main":           "",
		"^1.0@dev":           "",
		">=6.6.0.0 <6.8.0.0": "vers:composer/>=6.6.0.0|<6.8.0.0",
	}

	for constraint, expected := range cases {
		assert.Equal(t, expected, composerConstraintToVers(constraint), constraint)
	}
}

func TestNpmIntegrityToHash(t *testing.T) {
	hash, ok := npmIntegrityToHash("sha1-qUJyRqiXYXgNMRhClb0rtLc3k/c=")
	require.True(t, ok)
	assert.Equal(t, "SHA-1", hash.Alg)
	assert.Equal(t, "a9427246a89761780d31184295bd2bb4b73793f7", hash.Content)

	_, ok = npmIntegrityToHash("md5-abc")
	assert.False(t, ok)

	_, ok = npmIntegrityToHash("")
	assert.False(t, ok)
}
