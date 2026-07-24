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
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

const testComposerLock = `{
	"packages": [
		{"name": "shopware/core", "version": "v6.6.10.3"},
		{"name": "swag/demo", "version": "2.0.0", "type": "shopware-platform-plugin"}
	],
	"packages-dev": []
}`

func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "composer.json"), `{
		"name": "shopware/production",
		"require": {"shopware/core": "6.6.10.3", "shopware/deployment-helper": "*", "swag/demo": "^2.0"}
	}`)
	writeFile(t, filepath.Join(dir, "composer.lock"), testComposerLock)

	writeFile(t, filepath.Join(dir, "vendor", "swag", "demo", "composer.json"), `{
		"name": "swag/demo",
		"type": "shopware-platform-plugin",
		"version": "2.0.0",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-plugin-class": "Swag\\Demo\\Demo", "label": {"en-GB": "Demo"}},
		"autoload": {"psr-4": {"Swag\\Demo\\": "src/"}}
	}`)

	writeFile(t, filepath.Join(dir, "custom", "plugins", "LocalPlugin", "composer.json"), `{
		"name": "acme/local-plugin",
		"type": "shopware-platform-plugin",
		"version": "1.0.0",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-plugin-class": "Acme\\LocalPlugin\\LocalPlugin", "label": {"en-GB": "Local"}},
		"autoload": {"psr-4": {"Acme\\LocalPlugin\\": "src/"}}
	}`)

	return dir
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
	writeFile(t, filepath.Join(dir, "composer.json"), `{"require": {}}`)

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	lock := checkByID(t, r.Checks, "composer-lock")
	assert.Equal(t, StateFail, lock.State)
	assert.True(t, lock.Failed())
	assert.True(t, r.Blocked())
	assert.Nil(t, r.CurrentVersion)
}

func TestReadinessLockWithoutCore(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "composer.json"), `{"require": {}}`)
	writeFile(t, filepath.Join(dir, "composer.lock"), `{"packages": [], "packages-dev": []}`)

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
	writeFile(t, filepath.Join(dir, "composer.json"), `{"require": {"shopware/core": "6.6.10.3"}}`)

	r := newTestUpgrader(t, dir).RunReadinessChecks(t.Context())

	dh := checkByID(t, r.Checks, "deployment-helper")
	assert.Equal(t, StateWarn, dh.State)
	assert.False(t, dh.Failed(), "missing deployment helper does not block")
	assert.False(t, r.Blocked())
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
	writeFile(t, filepath.Join(dir, "a.txt"), "hello")

	check = checkGitClean(t.Context(), dir)
	assert.Equal(t, StateFail, check.State, "untracked file means dirty tree")
	assert.True(t, check.Failed())

	runGit("add", ".")
	runGit("commit", "-m", "init")

	check = checkGitClean(t.Context(), dir)
	assert.Equal(t, StateOK, check.State)
}
