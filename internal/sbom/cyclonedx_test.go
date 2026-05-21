package sbom

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestGenerate(t *testing.T) {
	lock := &packagist.ComposerLock{
		Packages: []packagist.ComposerLockPackage{
			{
				Name:        "symfony/console",
				Version:     "v6.3.0",
				Type:        "library",
				Description: "Eases the creation of beautiful and testable command line interfaces",
				Homepage:    "https://symfony.com",
				License:     []string{"MIT"},
				Require: map[string]string{
					"php":            ">=8.1",
					"symfony/string": "^6.3",
				},
				Dist: packagist.ComposerLockPackageDist{
					Type:   "zip",
					URL:    "https://api.github.com/repos/symfony/console/zipball/abc",
					Shasum: "abcdef0123456789",
				},
				Source: packagist.ComposerLockPackageSource{
					Type: "git",
					URL:  "https://github.com/symfony/console.git",
				},
			},
			{
				Name:    "symfony/string",
				Version: "v6.3.0",
				Type:    "library",
				License: []string{"MIT"},
				Require: map[string]string{"php": ">=8.1"},
			},
		},
		PackagesDev: []packagist.ComposerLockPackage{
			{Name: "phpunit/phpunit", Version: "10.0.0", License: []string{"BSD-3-Clause"}},
		},
	}

	t.Run("excludes dev dependencies by default", func(t *testing.T) {
		bom, err := Generate(lock, Options{ApplicationName: "acme/shop", ApplicationVersion: "1.0.0", ToolVersion: "test"})
		assert.NoError(t, err)
		assert.NotNil(t, bom)

		assert.Equal(t, "CycloneDX", bom.BOMFormat)
		assert.Equal(t, "1.5", bom.SpecVersion)
		assert.True(t, strings.HasPrefix(bom.SerialNumber, "urn:uuid:"))
		assert.Equal(t, 1, bom.Version)

		assert.NotNil(t, bom.Metadata.Component)
		assert.Equal(t, "application", bom.Metadata.Component.Type)
		assert.Equal(t, "acme/shop", bom.Metadata.Component.Name)
		assert.Equal(t, "1.0.0", bom.Metadata.Component.Version)

		assert.Len(t, bom.Components, 2)

		var consoleComponent Component
		for _, c := range bom.Components {
			if c.Name == "console" {
				consoleComponent = c
				break
			}
		}

		assert.Equal(t, "library", consoleComponent.Type)
		assert.Equal(t, "symfony", consoleComponent.Group)
		assert.Equal(t, "v6.3.0", consoleComponent.Version)
		assert.Equal(t, "pkg:composer/symfony/console@v6.3.0", consoleComponent.PURL)
		assert.Equal(t, "pkg:composer/symfony/console@v6.3.0", consoleComponent.BOMRef)
		assert.Len(t, consoleComponent.Licenses, 1)
		assert.Equal(t, "MIT", consoleComponent.Licenses[0].License.ID)
		assert.Len(t, consoleComponent.Hashes, 1)
		assert.Equal(t, "SHA-1", consoleComponent.Hashes[0].Alg)
		assert.Equal(t, "abcdef0123456789", consoleComponent.Hashes[0].Content)

		referenceTypes := []string{}
		for _, ref := range consoleComponent.ExternalReferences {
			referenceTypes = append(referenceTypes, ref.Type)
		}
		assert.ElementsMatch(t, []string{"website", "vcs", "distribution"}, referenceTypes)
	})

	t.Run("includes dev dependencies when requested", func(t *testing.T) {
		bom, err := Generate(lock, Options{IncludeDevDependencies: true})
		assert.NoError(t, err)
		assert.Len(t, bom.Components, 3)

		names := make([]string, 0, len(bom.Components))
		for _, c := range bom.Components {
			names = append(names, c.Group+"/"+c.Name)
		}
		assert.Contains(t, names, "phpunit/phpunit")
	})

	t.Run("dependencies link composer packages and skip platform requirements", func(t *testing.T) {
		bom, err := Generate(lock, Options{ApplicationName: "acme/shop"})
		assert.NoError(t, err)

		var consoleDeps Dependency
		var rootDeps Dependency
		for _, dep := range bom.Dependencies {
			if dep.Ref == "pkg:composer/symfony/console@v6.3.0" {
				consoleDeps = dep
			}
			if dep.Ref == "app:acme/shop" {
				rootDeps = dep
			}
		}

		assert.Equal(t, []string{"pkg:composer/symfony/string@v6.3.0"}, consoleDeps.DependsOn)
		assert.ElementsMatch(t,
			[]string{"pkg:composer/symfony/console@v6.3.0", "pkg:composer/symfony/string@v6.3.0"},
			rootDeps.DependsOn,
		)
	})

	t.Run("nil lock returns error", func(t *testing.T) {
		bom, err := Generate(nil, Options{})
		assert.Error(t, err)
		assert.Nil(t, bom)
	})
}

func TestMarshalProducesValidCycloneDXJSON(t *testing.T) {
	bom, err := Generate(&packagist.ComposerLock{
		Packages: []packagist.ComposerLockPackage{
			{Name: "symfony/console", Version: "v6.3.0", License: []string{"MIT"}},
		},
	}, Options{ApplicationName: "shop", ToolVersion: "1.0.0"})
	assert.NoError(t, err)

	data, err := Marshal(bom)
	assert.NoError(t, err)

	roundTrip := map[string]interface{}{}
	assert.NoError(t, json.Unmarshal(data, &roundTrip))

	assert.Equal(t, "CycloneDX", roundTrip["bomFormat"])
	assert.Equal(t, "1.5", roundTrip["specVersion"])
}

func TestIsPlatformPackage(t *testing.T) {
	platform := []string{"php", "hhvm", "ext-mbstring", "lib-curl", "composer-runtime-api", "composer-plugin-api"}
	notPlatform := []string{"symfony/console", "shopware/core", "composer/semver"}

	for _, name := range platform {
		assert.True(t, isPlatformPackage(name), "expected %q to be a platform package", name)
	}
	for _, name := range notPlatform {
		assert.False(t, isPlatformPackage(name), "expected %q not to be a platform package", name)
	}
}

func TestSplitComposerName(t *testing.T) {
	group, name := splitComposerName("symfony/console")
	assert.Equal(t, "symfony", group)
	assert.Equal(t, "console", name)

	group, name = splitComposerName("standalone")
	assert.Equal(t, "", group)
	assert.Equal(t, "standalone", name)
}

func TestLicensesFromPackageMapsSPDXAndFreeText(t *testing.T) {
	licenses := licensesFromPackage([]string{"MIT", "proprietary", "  ", "BSD-3-Clause"})

	assert.Len(t, licenses, 3)

	idMatches := make(map[string]bool)
	nameMatches := make(map[string]bool)
	for _, l := range licenses {
		if l.License.ID != "" {
			idMatches[l.License.ID] = true
		}
		if l.License.Name != "" {
			nameMatches[l.License.Name] = true
		}
	}

	assert.True(t, idMatches["MIT"], "MIT should be SPDX id")
	assert.True(t, idMatches["BSD-3-Clause"], "BSD-3-Clause should be SPDX id")
	assert.True(t, nameMatches["proprietary"], "proprietary should be free-text name")
}

func TestNewSerialNumberIsUUIDv4Like(t *testing.T) {
	serial, err := newSerialNumber()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(serial, "urn:uuid:"))

	// urn:uuid: prefix (9 chars) + 8-4-4-4-12 hex chars
	uuid := strings.TrimPrefix(serial, "urn:uuid:")
	parts := strings.Split(uuid, "-")
	assert.Len(t, parts, 5)
	assert.Len(t, parts[0], 8)
	assert.Len(t, parts[1], 4)
	assert.Len(t, parts[2], 4)
	assert.Len(t, parts[3], 4)
	assert.Len(t, parts[4], 12)
	assert.Equal(t, byte('4'), parts[2][0], "version nibble should be 4")
}
