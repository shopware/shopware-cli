package projectupgrade

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReleaseNotesURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://github.com/shopware/shopware/releases/tag/v6.6.4.0", ReleaseNotesURL("6.6.4.0"))
	assert.Equal(t, "https://github.com/shopware/shopware/releases/tag/v6.6.4.0", ReleaseNotesURL("v6.6.4.0"), "an existing v prefix is not doubled")
}

func TestBuildMarkdownReportGroupsExtensions(t *testing.T) {
	t.Parallel()

	report := BuildMarkdownReport(ReportData{
		CurrentVersion: "6.5.8.0",
		TargetVersion:  "6.6.4.0",
		Environment:    "docker",
		PHPConstraint:  ">=8.2",
		Preflight: []PreflightResult{
			{Label: "composer.lock present", Status: PreflightOK},
			{Label: "Git working tree clean", Status: PreflightSkipped, Detail: "skipped via --allow-dirty"},
		},
		Extensions: []ExtensionRow{
			{Name: "swag/blocked", Current: "3.0.0", State: ExtensionBlocked, Result: "no compatible release"},
			{Name: "swag/update", Current: "2.0.0", Target: "2.5.0", State: ExtensionUpdate, Result: "will be updated"},
			{Name: "swag/removed", Current: "1.0.0", State: ExtensionRemove, Result: "will be removed during the upgrade"},
			{Name: "swag/ok", Current: "1.0.0", Target: "1.0.0", State: ExtensionOK, Result: "compatible as installed"},
		},
		ComposerJSON:   `{"require": {}}`,
		ComposerOutput: []string{"Your requirements could not be resolved to an installable set of packages."},
	})

	assert.Contains(t, report, "Current Shopware version: `6.5.8.0`")
	assert.Contains(t, report, "releases/tag/v6.6.4.0")
	assert.Contains(t, report, "PHP requirement of Shopware 6.6.4.0: `>=8.2`")

	// The three blocks required by the issue.
	assert.Contains(t, report, "## Extensions: OK")
	assert.Contains(t, report, "## Extensions: Needed review")
	assert.Contains(t, report, "## Extensions: Was blocked")

	// Rows land in the right block (ordering: blocked block comes last).
	okIdx := indexOf(t, report, "## Extensions: OK")
	reviewIdx := indexOf(t, report, "## Extensions: Needed review")
	blockedIdx := indexOf(t, report, "## Extensions: Was blocked")
	assert.Less(t, okIdx, reviewIdx)
	assert.Less(t, reviewIdx, blockedIdx)

	assert.Contains(t, report[reviewIdx:blockedIdx], "swag/update")
	assert.Contains(t, report[reviewIdx:blockedIdx], "swag/removed")
	assert.Contains(t, report[blockedIdx:], "swag/blocked")

	// composer.json and the raw composer output are attached because a
	// blocker exists.
	assert.Contains(t, report, "## composer.json")
	assert.Contains(t, report, "## Raw composer output")
	assert.Contains(t, report, "Your requirements could not be resolved")
}

func TestBuildMarkdownReportOmitsRawComposerOutputWithoutBlockers(t *testing.T) {
	t.Parallel()

	report := BuildMarkdownReport(ReportData{
		CurrentVersion: "6.5.8.0",
		TargetVersion:  "6.6.4.0",
		Extensions: []ExtensionRow{
			{Name: "swag/ok", Current: "1.0.0", Target: "1.0.0", State: ExtensionOK, Result: "compatible as installed"},
		},
		ComposerOutput: []string{"Lock file operations: 0 installs, 2 updates, 0 removals"},
	})

	assert.NotContains(t, report, "## Raw composer output")
	assert.Contains(t, report, "None.", "empty blocks state that nothing was found")
}

func indexOf(t *testing.T, haystack, needle string) int {
	t.Helper()
	idx := strings.Index(haystack, needle)
	if idx < 0 {
		t.Fatalf("expected %q in report", needle)
	}
	return idx
}
