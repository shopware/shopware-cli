package projectupgrade

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckComposerLock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	r := checkComposerLock(dir)
	assert.Equal(t, PreflightFailed, r.Status)
	assert.NotEmpty(t, r.Explanation)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte("{}"), 0o644))
	r = checkComposerLock(dir)
	assert.Equal(t, PreflightOK, r.Status)
}

func TestCheckGitCleanSkippedViaAllowDirty(t *testing.T) {
	t.Parallel()

	r := checkGitClean(t.Context(), t.TempDir(), true)
	assert.Equal(t, PreflightSkipped, r.Status)
}

func TestCheckGitCleanNonRepoIsOK(t *testing.T) {
	t.Parallel()

	r := checkGitClean(t.Context(), t.TempDir(), false)
	assert.Equal(t, PreflightOK, r.Status)
	assert.Equal(t, "not a git repository", r.Detail)
}

func TestCheckGitCleanDirtyRepoFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gitCmd := exec.CommandContext(t.Context(), "git", "-C", dir, "init")
	require.NoError(t, gitCmd.Run())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644))

	r := checkGitClean(t.Context(), dir, false)
	assert.Equal(t, PreflightFailed, r.Status)
	assert.Contains(t, r.Explanation, "--allow-dirty")
}

func TestCheckComposerManagedPlugins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// No custom/plugins directory at all: nothing to complain about.
	r := checkComposerManagedPlugins(dir, false)
	assert.Equal(t, PreflightOK, r.Status)

	// An orphan plugin directory not tracked by composer fails the check.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "custom", "plugins", "MyOrphan"), 0o755))
	r = checkComposerManagedPlugins(dir, false)
	assert.Equal(t, PreflightFailed, r.Status)
	assert.Contains(t, r.Explanation, "MyOrphan")

	// The override flag downgrades it to skipped.
	r = checkComposerManagedPlugins(dir, true)
	assert.Equal(t, PreflightSkipped, r.Status)
}

func TestCheckEnvironmentRunningWithoutExecutor(t *testing.T) {
	t.Parallel()

	r := checkEnvironmentRunning(t.Context(), nil)
	assert.Equal(t, PreflightSkipped, r.Status)
}

func TestCheckPackagistReachable(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failServer.Close()

	orig := packagistPingURL
	defer func() { packagistPingURL = orig }()

	packagistPingURL = okServer.URL
	r := checkPackagistReachable(t.Context())
	assert.Equal(t, PreflightOK, r.Status)

	packagistPingURL = failServer.URL
	r = checkPackagistReachable(t.Context())
	assert.Equal(t, PreflightFailed, r.Status)

	packagistPingURL = "http://127.0.0.1:1"
	r = checkPackagistReachable(t.Context())
	assert.Equal(t, PreflightFailed, r.Status)
	assert.Contains(t, r.Explanation, "Composer needs to reach")
}

func TestPreflightBlocked(t *testing.T) {
	t.Parallel()

	assert.False(t, PreflightBlocked(nil))
	assert.False(t, PreflightBlocked([]PreflightResult{{Status: PreflightOK}, {Status: PreflightSkipped}}))
	assert.True(t, PreflightBlocked([]PreflightResult{{Status: PreflightOK}, {Status: PreflightFailed}}))
}
