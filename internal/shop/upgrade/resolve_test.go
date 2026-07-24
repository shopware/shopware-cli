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
	assert.Equal(t, "6.7.11.0", result.ResolvedVersion("shopware/core"))
	assert.Empty(t, result.ResolvedVersion("vendor/untouched"))
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
