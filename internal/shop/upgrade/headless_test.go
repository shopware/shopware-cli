package upgrade

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
)

// headlessUpgrader builds an upgrader whose network lookups are stubbed and
// whose composer invocations run composerScript (version probes always
// succeed).
func headlessUpgrader(t *testing.T, dir, composerScript string) *ProjectUpgrader {
	t.Helper()

	exec := &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			if len(args) > 0 && args[0] == "--version" {
				return shellProcess(ctx, "true")
			}
			return shellProcess(ctx, composerScript)
		},
		php: func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "true") },
	}

	u := NewProjectUpgrader(dir, exec)
	u.shopwareVersions = func(context.Context) ([]string, error) {
		return []string{"6.6.10.19", "6.7.11.0"}, nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)
	u.endOfLifeURL = server.URL
	u.repositories = func(*composer.Json, *composer.Auth) *repository.Set {
		return repository.NewSet()
	}
	u.extensionUpdates = func(context.Context, string, string, []account_api.UpdateCheckExtension) ([]account_api.UpdateCheckExtensionCompatibility, error) {
		return nil, nil
	}
	return u
}

func headlessCatalog() *Catalog {
	return &Catalog{
		Current: version.Must(version.NewVersion("6.6.10.3")),
		Options: []VersionOption{
			{Version: version.Must(version.NewVersion("6.7.11.0")), Tag: "recommended"},
			{Version: version.Must(version.NewVersion("6.6.10.19")), Tag: "latest 6.6 patch"},
		},
		Recommended: 0,
		LatestPatch: 1,
	}
}

func TestSelectTarget(t *testing.T) {
	catalog := headlessCatalog()

	_, err := selectTarget(catalog, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target is required")
	assert.Contains(t, err.Error(), "6.7.11.0  (recommended)")
	assert.Contains(t, err.Error(), "--target recommended / --target latest-patch")

	opt, err := selectTarget(catalog, TargetRecommended)
	require.NoError(t, err)
	assert.Equal(t, "6.7.11.0", opt.Version.String())

	opt, err = selectTarget(catalog, TargetLatestPatch)
	require.NoError(t, err)
	assert.Equal(t, "6.6.10.19", opt.Version.String())

	opt, err = selectTarget(catalog, "v6.7.11.0")
	require.NoError(t, err)
	assert.Equal(t, "6.7.11.0", opt.Version.String(), "a v prefix is accepted")

	_, err = selectTarget(catalog, "6.5.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a supported upgrade target")
}

func TestRunHeadlessRequiresTarget(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := setupProject(t)
	u := headlessUpgrader(t, dir, "true")

	var out bytes.Buffer
	err := u.RunHeadless(t.Context(), HeadlessOptions{Out: &out})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target is required")
	assert.Contains(t, ansi.Strip(out.String()), "Readiness checks")
}

func TestRunHeadlessDryRun(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := setupProject(t)
	before, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	u := headlessUpgrader(t, dir, "true")

	var out bytes.Buffer
	err = u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", DryRun: true, Out: &out})
	require.NoError(t, err)

	assert.Contains(t, ansi.Strip(out.String()), "Upgrade Shopware 6.6.10.3 -> 6.7.11.0")
	assert.Contains(t, ansi.Strip(out.String()), "Composer can resolve this upgrade.")
	assert.Contains(t, ansi.Strip(out.String()), "Planned changes (dry run — nothing modified)")
	assert.Contains(t, ansi.Strip(out.String()), "composer update --with-all-dependencies")

	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a dry run must not modify the project")

	_, err = os.Stat(u.ReportPath())
	assert.NoError(t, err, "the dry run writes the report")
}

func TestRunHeadlessSecurityBlockedNeedsOptIn(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := setupProject(t)
	u := headlessUpgrader(t, dir, "echo 'these were not loaded, because they are affected by security advisories'; exit 2")

	var out bytes.Buffer
	err := u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", Out: &out})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-run with --no-audit")
	assert.Contains(t, err.Error(), "Shopware Security plugin")
	assert.False(t, u.AuditBlockDisabled())
}

func TestRunHeadlessExecutes(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := setupProject(t)
	u := headlessUpgrader(t, dir, "true")

	var out bytes.Buffer
	err := u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", Out: &out})
	require.NoError(t, err)

	assert.Contains(t, ansi.Strip(out.String()), "Executing upgrade")
	assert.Contains(t, ansi.Strip(out.String()), "Upgraded to Shopware 6.7.11.0.")

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"shopware/core": "6.7.11.0"`, "composer.json is rewritten to the target")

	_, err = os.Stat(u.ReportPath())
	assert.NoError(t, err)
	_, err = os.Stat(u.LogPath())
	assert.NoError(t, err)
}

func TestRunHeadlessFailingUpdateRollsBack(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := setupProject(t)
	before, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	// The preflight resolution must succeed while the real `composer update`
	// fails, so fail only when the runner's --with-all-dependencies run
	// executes without the --no-install preflight flag.
	u := headlessUpgrader(t, dir, `case "$*" in *--no-install*) exit 0;; *) echo boom >&2; exit 2;; esac`)
	u.executor.(*fakeExecutor).composer = func(ctx context.Context, args ...string) *executor.Process {
		if len(args) > 0 && args[0] == "--version" {
			return shellProcess(ctx, "true")
		}
		for _, a := range args {
			if a == "--no-install" {
				return shellProcess(ctx, "true")
			}
		}
		return shellProcess(ctx, "echo boom >&2; exit 2")
	}

	var out bytes.Buffer
	err = u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", Out: &out})

	require.Error(t, err)
	assert.Contains(t, ansi.Strip(out.String()), "Upgrade failed and was rolled back.")

	after, readErr := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, readErr)
	assert.Equal(t, string(before), string(after), "composer.json is restored after the failure")
}
