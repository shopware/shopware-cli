package upgrade

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReport(t *testing.T) {
	dir := t.TempDir()

	path, err := newTestUpgrader(t, dir).WriteReport(ReportData{
		ProjectName: "acme-shop",
		Current:     "6.6.10.3",
		Target:      "6.7.11.0",
		GeneratedAt: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		Checks: []ReadinessCheck{
			{Label: "Git working tree clean", Value: "yes", State: StateOK},
		},
		PlannedChanges: []string{"shopware/core: 6.6.10.3 -> 6.7.11.0"},
		PHPRequirement: ">=8.2",
		PHPInstalled:   "8.3.6",
		Extensions: []ExtensionResult{
			{Extension: InstalledExtension{Name: "SwagOk", Package: "swag/ok", Version: "2.0.0"}, Status: ExtOK, Available: "2.0.0"},
			{Extension: InstalledExtension{Name: "LocalPlugin", Version: "1.0.0"}, Status: ExtReview, Detail: "Local extension"},
			{Extension: InstalledExtension{Name: "AcmeBlocked", Package: "acme/blocked", Version: "3.0.0"}, Status: ExtBlocked, Detail: "No compatible release"},
		},
		ComposerReport: "Your requirements could not be resolved.",
		ResolvedChanges: []PackageChange{
			{Name: "shopware/core", From: "6.6.10.3", To: "6.7.11.0", Op: "upgrade"},
			{Name: "shopware/deployment-helper", To: "0.5.1", Op: "install"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, ".shopware-cli", "upgrade", "report.md"), path)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	report := string(content)

	assert.Contains(t, report, "# Shopware upgrade report — acme-shop")
	assert.Contains(t, report, "**From:** Shopware 6.6.10.3")
	assert.Contains(t, report, "**To:** Shopware 6.7.11.0")
	assert.Contains(t, report, "| PHP >=8.2 | installed: 8.3.6 |")
	assert.Contains(t, report, "`shopware/core: 6.6.10.3 -> 6.7.11.0`")

	assert.Contains(t, report, "## Extensions: Blocked (1)")
	assert.Contains(t, report, "## Extensions: Needs review (1)")
	assert.Contains(t, report, "## Extensions: OK (1)")
	assert.Less(t, indexOf(report, "AcmeBlocked"), indexOf(report, "LocalPlugin"), "blocked group is listed first")

	assert.Contains(t, report, "Your requirements could not be resolved.")

	assert.Contains(t, report, "## Resolved package changes")
	assert.Contains(t, report, "| shopware/core | 6.6.10.3 | 6.7.11.0 | upgrade |")
	assert.Contains(t, report, "| shopware/deployment-helper | — | 0.5.1 | install |")
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestWriteReportSkipsEmptyGroups(t *testing.T) {
	dir := t.TempDir()

	path, err := newTestUpgrader(t, dir).WriteReport(ReportData{
		ProjectName: "shop",
		Current:     "6.6.10.3",
		Target:      "6.7.11.0",
		Extensions: []ExtensionResult{
			{Extension: InstalledExtension{Name: "SwagOk"}, Status: ExtOK},
		},
	})
	require.NoError(t, err)

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(content), "Blocked")
	assert.NotContains(t, string(content), "Composer resolution report")
}
