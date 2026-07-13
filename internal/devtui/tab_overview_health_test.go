package devtui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/symfony"
)

func TestParsePHPMemoryLimit(t *testing.T) {
	cases := []struct {
		value string
		bytes int64
		ok    bool
	}{
		{"512M", 512 * 1024 * 1024, true},
		{"1G", 1024 * 1024 * 1024, true},
		{"131072K", 128 * 1024 * 1024, true},
		{"134217728", 128 * 1024 * 1024, true},
		{"-1", -1, true},
		{" 256m ", 256 * 1024 * 1024, true},
		{"", 0, false},
		{"abc", 0, false},
	}

	for _, tc := range cases {
		bytes, ok := parsePHPMemoryLimit(tc.value)
		assert.Equal(t, tc.ok, ok, tc.value)
		if tc.ok {
			assert.Equal(t, tc.bytes, bytes, tc.value)
		}
	}
}

func TestMemoryLimitCheck(t *testing.T) {
	assert.Equal(t, healthOK, memoryLimitCheck("512M").Level)
	assert.Equal(t, healthOK, memoryLimitCheck("1G").Level)
	// -1 means unlimited in php.ini.
	assert.Equal(t, healthOK, memoryLimitCheck("-1").Level)
	assert.Equal(t, healthWarn, memoryLimitCheck("128M").Level)
	assert.Equal(t, healthWarn, memoryLimitCheck("garbage").Level)
}

func writeComposerLock(t *testing.T, dir, phpConstraint string) {
	t.Helper()
	lock := `{"packages":[{"name":"shopware/core","version":"v6.7.0.0","require":{"php":"` + phpConstraint + `"}}]}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
}

func TestPHPVersionCheck(t *testing.T) {
	dir := t.TempDir()
	writeComposerLock(t, dir, ">=8.2")

	check := phpVersionCheck(dir, "8.3.12")
	assert.Equal(t, healthOK, check.Level)
	assert.Equal(t, "8.3.12", check.Current)
	assert.Equal(t, ">=8.2", check.Recommended)

	assert.Equal(t, healthCritical, phpVersionCheck(dir, "8.1.0").Level)
}

func TestPHPVersionCheck_NoComposerLock(t *testing.T) {
	check := phpVersionCheck(t.TempDir(), "8.3.12")
	assert.Equal(t, healthOK, check.Level)
	assert.Equal(t, "-", check.Recommended)
}

func TestAdminWorkerCheck(t *testing.T) {
	disabled := adminWorkerCheck(false)
	assert.Equal(t, healthOK, disabled.Level)
	assert.Equal(t, "disabled", disabled.Current)

	enabled := adminWorkerCheck(true)
	assert.Equal(t, healthWarn, enabled.Level)
	assert.Equal(t, "enabled", enabled.Current)
}

func writeMonologConfig(t *testing.T, dir, level string) {
	t.Helper()
	packages := filepath.Join(dir, "config", "packages")
	require.NoError(t, os.MkdirAll(packages, 0o755))
	yaml := "monolog:\n    handlers:\n        business_event_handler_buffer:\n            level: " + level + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(packages, "monolog.yaml"), []byte(yaml), 0o644))
}

func TestFlowBuilderLogLevelCheck(t *testing.T) {
	dir := t.TempDir()
	writeMonologConfig(t, dir, "warning")

	pc, err := symfony.NewProjectConfig(dir)
	require.NoError(t, err)

	check := flowBuilderLogLevelCheck(pc, "dev")
	assert.Equal(t, healthOK, check.Level)
	assert.Equal(t, "WARNING", check.Current)
}

func TestFlowBuilderLogLevelCheck_DefaultsToDebug(t *testing.T) {
	pc, err := symfony.NewProjectConfig(t.TempDir())
	require.NoError(t, err)

	check := flowBuilderLogLevelCheck(pc, "dev")
	assert.Equal(t, healthWarn, check.Level)
	assert.Equal(t, "DEBUG", check.Current)
}

func TestCollectSetupHealth_WithoutExecutor(t *testing.T) {
	checks := collectSetupHealth(t.Context(), t.TempDir(), nil)

	// No runtime checks without an executor, but the config-derived checks
	// still report their defaults (admin worker enabled, log level debug).
	names := make([]string, 0, len(checks))
	for _, c := range checks {
		names = append(names, c.Name)
	}
	assert.ElementsMatch(t, []string{"Admin Worker", "Flow Builder log level"}, names)
}

func TestOverviewViewShowsSetupHealth(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil, nil)
	m.loading = false
	m.healthLoading = false
	m.health = []healthCheck{
		{Group: healthGroupRuntime, Name: "PHP version", Current: "8.3.12", Recommended: ">= 8.2", Level: healthOK},
		{Group: healthGroupDebug, Name: "Flow Builder log level", Current: "DEBUG", Recommended: "min WARNING", Level: healthWarn},
	}

	for _, width := range []int{120, 80} { // two-column and stacked layouts
		view := m.View(width, 40)
		assert.Contains(t, view, "Setup health")
		assert.Contains(t, view, "Runtime")
		assert.Contains(t, view, "PHP version")
		assert.Contains(t, view, "8.3.12")
		assert.Contains(t, view, ">= 8.2")
		assert.Contains(t, view, "Debug (Flow Builder)")
		assert.Contains(t, view, "min WARNING")
		assert.Contains(t, view, "Watchers")
	}

	// The "User action" column heading only exists in the two-column layout.
	assert.Contains(t, m.View(120, 40), "User action")
	assert.NotContains(t, m.View(80, 40), "User action")
}

func TestSetupHealthLinksAreZeroWidth(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil, nil)
	m.loading = false
	m.healthLoading = false
	m.health = []healthCheck{
		{Group: healthGroupRuntime, Name: "PHP version", Current: "8.3.12", Recommended: ">= 8.2", Level: healthOK, DocsURL: hostingDocsURL},
		{Group: healthGroupRuntime, Name: "Memory limit", Current: "512M", Recommended: ">= 512M", Level: healthOK},
	}

	report := m.renderSetupHealth()
	assert.Contains(t, report, "\x1b]8;;"+hostingDocsURL)

	// The hyperlink escape sequence must not shift the Current column: both
	// rows (one linked, one not) align their values at the same offset.
	var phpLine, memLine string
	for _, line := range strings.Split(stripANSI(report), "\n") {
		if strings.Contains(line, "PHP version") {
			phpLine = line
		}
		if strings.Contains(line, "Memory limit") {
			memLine = line
		}
	}
	require.NotEmpty(t, phpLine)
	require.NotEmpty(t, memLine)
	assert.Equal(t, strings.Index(phpLine, "8.3.12"), strings.Index(memLine, "512M"))
}

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m|\x1b\]8;;[^\x1b]*\x1b\\`)

func stripANSI(s string) string {
	return ansiRe.ReplaceAllString(s, "")
}

func TestOverviewViewSetupHealthLoading(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil, nil)
	m.loading = false

	assert.Contains(t, m.View(120, 40), "CHECKING")

	updated, _ := m.Update(setupHealthLoadedMsg{})
	assert.False(t, updated.healthLoading)
	assert.Contains(t, updated.View(120, 40), "No setup checks available.")
}
