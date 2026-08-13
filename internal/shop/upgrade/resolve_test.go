package upgrade

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLocks(t *testing.T) (currentPath, resolvedPath string) {
	t.Helper()
	dir := t.TempDir()
	currentPath = filepath.Join(dir, "composer.lock")
	resolvedPath = filepath.Join(dir, "resolved.lock")

	writeFile(t, currentPath, `{
		"packages": [
			{"name": "shopware/core", "version": "v6.6.10.3"},
			{"name": "swag/demo", "version": "2.0.0", "type": "shopware-platform-plugin"},
			{"name": "acme/other", "version": "2.0.0"},
			{"name": "legacy/package", "version": "1.0.0"},
			{"name": "vendor/untouched", "version": "1.0.0"}
		],
		"packages-dev": []
	}`)
	writeFile(t, resolvedPath, `{
		"packages": [
			{"name": "shopware/core", "version": "v6.7.11.0"},
			{"name": "shopware/deployment-helper", "version": "v0.5.1"},
			{"name": "swag/demo", "version": "2.1.3", "type": "shopware-platform-plugin"},
			{"name": "acme/other", "version": "1.9.0"},
			{"name": "vendor/untouched", "version": "1.0.0"}
		],
		"packages-dev": []
	}`)
	return currentPath, resolvedPath
}

func TestDiffLocks(t *testing.T) {
	currentPath, resolvedPath := writeLocks(t)

	changes := diffLocks(currentPath, resolvedPath)
	require.Len(t, changes, 5)

	assert.Equal(t, []PackageChange{
		{Name: "acme/other", From: "2.0.0", To: "1.9.0", Op: "downgrade"},
		{Name: "legacy/package", From: "1.0.0", Op: "remove"},
		{Name: "shopware/core", From: "6.6.10.3", To: "6.7.11.0", Op: "upgrade"},
		{Name: "shopware/deployment-helper", To: "0.5.1", Op: "install"},
		{Name: "swag/demo", From: "2.0.0", To: "2.1.3", Op: "upgrade"},
	}, changes)
}

func TestDiffLocksMissingResolvedLock(t *testing.T) {
	currentPath, _ := writeLocks(t)
	assert.Nil(t, diffLocks(currentPath, filepath.Join(t.TempDir(), "missing.lock")))
}

func resolvedTestResult(t *testing.T) ResolveResult {
	t.Helper()
	currentPath, resolvedPath := writeLocks(t)
	return ResolveResult{OK: true, Changes: diffLocks(currentPath, resolvedPath)}
}

func TestResolvedVersion(t *testing.T) {
	result := resolvedTestResult(t)

	assert.Equal(t, "2.1.3", result.ResolvedVersion("swag/demo"))
	assert.Equal(t, "2.1.3", result.ResolvedVersion("Swag/Demo"), "Composer package names are case-insensitive")
	assert.Equal(t, "6.7.11.0", result.ResolvedVersion("shopware/core"))
	assert.Empty(t, result.ResolvedVersion("vendor/untouched"))
}

func TestLockVersionsIncludesDevelopmentPackages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "composer.lock")
	writeFile(t, path, `{
		"packages": [{"name": "shopware/core", "version": "v6.7.11.0"}],
		"packages-dev": [{"name": "phpunit/phpunit", "version": "v11.5.0"}]
	}`)

	assert.Equal(t, map[string]string{
		"shopware/core":   "6.7.11.0",
		"phpunit/phpunit": "11.5.0",
	}, lockVersions(path))
}

func TestApplyResolvedVersions(t *testing.T) {
	results := []ExtensionResult{
		{Extension: InstalledExtension{Name: "SwagDemo", Package: "swag/demo", Version: "2.0.0"}, Status: ExtNeedsUpdate, Available: "2.1.0"},
		{Extension: InstalledExtension{Name: "Untouched", Package: "vendor/untouched", Version: "1.0.0"}, Status: ExtOK, Available: "1.0.0"},
		{Extension: InstalledExtension{Name: "LocalPlugin", Version: "1.0.0"}, Status: ExtReview},
	}

	ApplyResolvedVersions(results, resolvedTestResult(t))

	assert.Equal(t, "2.1.3", results[0].Available, "metadata guess is replaced by the resolved release")
	assert.Equal(t, "1.0.0", results[1].Available, "unchanged packages keep their version")
	assert.Empty(t, results[2].Available, "local extensions stay untouched")
}

func TestResolveResultSecurityBlocked(t *testing.T) {
	blocked := ResolveResult{OK: false, Report: `- shopware/core v6.7.12.1 requires dompdf/dompdf 3.1.4 -> found dompdf/dompdf[v3.1.4] but these were not loaded, because they are affected by security advisories ("PKSA-cv56-2228-pzqx")`}
	assert.True(t, blocked.SecurityBlocked())

	conflict := ResolveResult{OK: false, Report: "requires shopware/core v6.7.6.0 but it conflicts with your root composer.json require"}
	assert.False(t, conflict.SecurityBlocked())

	ok := ResolveResult{OK: true, Report: "affected by security advisories"}
	assert.False(t, ok.SecurityBlocked(), "a successful resolution is never security-blocked")
}

func TestApplyResolvedVersionsOverridesMetadataBlockers(t *testing.T) {
	resolve := resolvedTestResult(t)
	require.True(t, resolve.OK)

	results := []ExtensionResult{
		// Not in the lock diff: the solver kept the installed release.
		{Extension: InstalledExtension{Name: "FroshTools", Package: "frosh/tools", Version: "3.12.0"}, Status: ExtBlocked},
		// In the diff: the solver picked a newer release.
		{Extension: InstalledExtension{Name: "SwagDemo", Package: "swag/demo", Version: "2.0.0"}, Status: ExtMismatch},
	}
	ApplyResolvedVersions(results, resolve)

	assert.Equal(t, ExtOK, results[0].Status, "a successful resolve disproves the metadata blocker")
	assert.Equal(t, "3.12.0", results[0].Available)
	assert.Contains(t, results[0].Detail, "installed release")
	assert.Contains(t, results[0].Detail, "Composer resolution determined the final compatibility result")
	assert.NotContains(t, results[0].Detail, "wrong or incomplete")

	assert.Equal(t, ExtNeedsUpdate, results[1].Status)
	assert.Equal(t, "2.1.3", results[1].Available)
	assert.Equal(t, "Composer resolution determined the final compatibility result.", results[1].Detail)
}

func TestApplyResolvedVersionsRemovedPackageStaysBlocked(t *testing.T) {
	// A transitively installed extension can be dropped by the solver; that
	// must not read as "kept at the installed release".
	results := []ExtensionResult{
		{Extension: InstalledExtension{Name: "SwagGone", Package: "swag/gone", Version: "1.0.0"}, Status: ExtBlocked, Available: "x"},
	}
	ApplyResolvedVersions(results, ResolveResult{OK: true, Changes: []PackageChange{
		{Name: "swag/gone", From: "1.0.0", Op: "remove"},
	}})

	assert.Equal(t, ExtBlocked, results[0].Status, "a removal is not compatibility")
	assert.Empty(t, results[0].Available)
	assert.Contains(t, results[0].Detail, "removes this package")
}

func TestApplyResolvedVersionsKeepsBlockersOnFailedResolve(t *testing.T) {
	results := []ExtensionResult{
		{Extension: InstalledExtension{Name: "SwagBlocked", Package: "swag/blocked", Version: "3.2.0"}, Status: ExtBlocked, Detail: "original"},
	}
	ApplyResolvedVersions(results, ResolveResult{OK: false})

	assert.Equal(t, ExtBlocked, results[0].Status, "a failed resolve proves nothing")
	assert.Equal(t, "original", results[0].Detail)
}
