package upgrade

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

var testComposerLock = testhelper.ComposerLock(
	testhelper.LockPackage{Name: "shopware/core", Version: "v6.6.10.3"},
	testhelper.LockPackage{Name: "swag/demo", Version: "2.0.0", Type: "shopware-platform-plugin"},
)

func setupPathPluginProject(t *testing.T) string {
	t.Helper()

	pathPlugin := testhelper.PluginComposer("acme/custom-plugin", "1.0.0", `Acme\MyCustomPlugin\MyCustomPlugin`)
	pathPlugin.Require = map[string]string{"shopware/core": "~6.7.0"}

	p := testhelper.NewProject(t).
		File("composer.json", testhelper.ComposerJSON{
			Name: "shopware/production",
			Require: map[string]string{
				"shopware/core":              "6.7.3.0",
				"shopware/deployment-helper": "*",
				"acme/custom-plugin":         "*",
			},
			Repositories: []map[string]any{
				{"type": "path", "url": "custom/static-plugins/*", "options": map[string]any{"symlink": true}},
			},
		}.String()).
		File("composer.lock", testhelper.ComposerLock(
			testhelper.LockPackage{Name: "shopware/core", Version: "v6.7.3.0"},
			testhelper.LockPackage{
				Name: "acme/custom-plugin", Version: "1.0.0", Type: "shopware-platform-plugin",
				Require: map[string]string{"shopware/core": "~6.7.0"},
				Dist:    map[string]string{"type": "path", "url": "custom/static-plugins/MyCustomPlugin"},
			},
		)).
		File("custom/static-plugins/MyCustomPlugin/composer.json", pathPlugin.String()).
		Dir("vendor/acme")

	require.NoError(t, os.Symlink(
		filepath.Join(p.Root, "custom", "static-plugins", "MyCustomPlugin"),
		filepath.Join(p.Root, "vendor", "acme", "custom-plugin"),
	))

	return p.Root
}

func setupProject(t *testing.T) string {
	t.Helper()

	p := testhelper.NewProject(t).
		File("composer.json", testhelper.ComposerJSON{
			Name:    "shopware/production",
			Require: map[string]string{"shopware/core": "6.6.10.3", "shopware/deployment-helper": "*", "swag/demo": "^2.0"},
		}.String()).
		File("composer.lock", testComposerLock).
		VendorPackage("swag/demo", testhelper.PluginComposer("swag/demo", "2.0.0", `Swag\Demo\Demo`)).
		CustomPlugin("LocalPlugin", testhelper.PluginComposer("acme/local-plugin", "1.0.0", `Acme\LocalPlugin\LocalPlugin`))

	return p.Root
}

func newTestUpgrader(t *testing.T, dir string) *ProjectUpgrader {
	t.Helper()
	exec := &fakeExecutor{
		composer: func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "true") },
		php:      func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "true") },
	}
	return NewProjectUpgrader(dir, exec)
}

func checkByID(t *testing.T, checks []ReadinessCheck, id string) ReadinessCheck {
	t.Helper()
	for _, c := range checks {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("check %q not found", id)
	return ReadinessCheck{}
}

func TestRunReadinessChecks(t *testing.T) {
	dir := setupProject(t)

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	require.NotNil(t, r.CurrentVersion)
	assert.Equal(t, "6.6.10.3", r.CurrentVersion.String())

	repo := checkByID(t, r.Checks, "repository")
	assert.Equal(t, StateOK, repo.State)
	assert.Equal(t, filepath.Base(dir), repo.Value)

	lock := checkByID(t, r.Checks, "composer-lock")
	assert.Equal(t, StateOK, lock.State)

	dh := checkByID(t, r.Checks, "deployment-helper")
	assert.Equal(t, StateOK, dh.State)

	// The local plugin is not Composer-managed: the upgrade cannot resolve
	// its version, so readiness blocks until it is required via Composer.
	ext := checkByID(t, r.Checks, "extensions")
	assert.Equal(t, StateFail, ext.State)
	assert.Equal(t, "1 of 2", ext.Value)
	assert.Contains(t, ext.Detail, "LocalPlugin")
	assert.Contains(t, ext.Detail, "autofix composer-plugins")
	assert.True(t, r.Blocked())

	names := make(map[string]bool)
	for _, e := range r.Extensions {
		names[e.Name] = e.ComposerManaged
	}
	assert.Equal(t, map[string]bool{"Demo": true, "LocalPlugin": false}, names)
}

func TestDiscoverExtensionsPathRepositoryIsComposerManaged(t *testing.T) {
	dir := setupPathPluginProject(t)

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	ext := checkByID(t, r.Checks, "extensions")
	assert.Equal(t, StateOK, ext.State)
	assert.Equal(t, "1 of 1", ext.Value)
	assert.False(t, r.Blocked())

	require.Len(t, r.Extensions, 1)
	assert.Equal(t, "MyCustomPlugin", r.Extensions[0].Name)
	assert.Equal(t, "acme/custom-plugin", r.Extensions[0].Package)
	assert.True(t, r.Extensions[0].ComposerManaged)
	assert.True(t, r.Extensions[0].PathInstalled)
	assert.Equal(t, "~6.7.0", r.Extensions[0].Require["shopware/core"])
}

func TestReadinessAllExtensionsComposerManaged(t *testing.T) {
	dir := setupProject(t)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "custom")))

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	ext := checkByID(t, r.Checks, "extensions")
	assert.Equal(t, StateOK, ext.State)
	assert.Equal(t, "1 of 1", ext.Value)
	assert.False(t, r.Blocked())
}

func TestReadinessMissingComposerLock(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), `{"require": {}}`)

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	lock := checkByID(t, r.Checks, "composer-lock")
	assert.Equal(t, StateFail, lock.State)
	assert.True(t, lock.Failed())
	assert.True(t, r.Blocked())
	assert.Nil(t, r.CurrentVersion)
}

func TestReadinessLockWithoutCore(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), `{"require": {}}`)
	testhelper.WriteFile(t, filepath.Join(dir, "composer.lock"), testhelper.ComposerLock())

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	lock := checkByID(t, r.Checks, "composer-lock")
	assert.Equal(t, StateFail, lock.State)
	assert.Contains(t, lock.Detail, "shopware/core")
}

func TestReadinessDeploymentHelperMissing(t *testing.T) {
	dir := setupProject(t)
	// Drop the local plugin so the extension gate does not block; this test
	// only cares about the deployment-helper check.
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "custom")))
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"),
		testhelper.ComposerJSON{Require: map[string]string{"shopware/core": "6.6.10.3"}}.String())

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	dh := checkByID(t, r.Checks, "deployment-helper")
	assert.Equal(t, StateWarn, dh.State)
	assert.False(t, dh.Failed(), "missing deployment helper does not block")
	assert.False(t, r.Blocked())
}

func TestNilExecutorChecksFailGracefully(t *testing.T) {
	u := NewProjectUpgrader(t.TempDir(), nil)

	tooling := u.checkTooling(t.Context())
	assert.Equal(t, StateFail, tooling.State)
	assert.Equal(t, "Executor is not initialized", tooling.Detail)
	assert.Empty(t, u.InstalledPHPVersion(t.Context()))
}

func TestCheckGitClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	dir := t.TempDir()

	check := checkGitClean(t.Context(), dir)
	assert.Equal(t, StateWarn, check.State, "no repository is a warning")
	assert.False(t, check.Blocking)

	runGit := func(args ...string) {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir, "-c", "commit.gpgsign=false"}, args...)...)
		// Isolate from the developer's global git config (signing, hooks, …).
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t.de",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t.de",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	runGit("init")
	testhelper.WriteFile(t, filepath.Join(dir, "a.txt"), "hello")

	check = checkGitClean(t.Context(), dir)
	assert.Equal(t, StateFail, check.State, "untracked file means dirty tree")
	assert.True(t, check.Failed())

	runGit("add", ".")
	runGit("commit", "-m", "init")

	check = checkGitClean(t.Context(), dir)
	assert.Equal(t, StateOK, check.State)
}
