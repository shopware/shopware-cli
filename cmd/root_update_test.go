package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/update"
)

// clearCIEnv blanks every CI marker internal/system/ci.go checks, so update
// gating behaves the same on developer machines and CI runners.
func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_URL", "BUILDKITE", "CIRCLECI", "DRONE", "TEAMCITY_VERSION", "TF_BUILD"} {
		t.Setenv(name, "")
	}
}

func TestCheckForUpdateSkipsForDevBuildAndNoUpdateHint(t *testing.T) {
	for _, args := range [][]string{nil, {"--no-update-hint"}, {"-n"}} {
		release, err := checkForUpdate(t.Context(), args)
		require.ErrorIs(t, err, update.ErrNoUpdateAvailable)
		assert.Nil(t, release)
	}
}

func TestCheckForUpdateReturnsCachedNewerRelease(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	t.Setenv("SHOPWARE_CLI_NO_UPDATE_NOTIFICATION", "")
	clearCIEnv(t)

	oldVersion := version
	version = "1.0.0"
	t.Cleanup(func() { version = oldVersion })

	// A fresh FetchedAt makes getReleaseInformation use the cache, so the
	// hard-coded release URL is never fetched.
	cached := update.ReleaseInfo{Version: "99.0.0", FetchedAt: time.Now()}
	data, err := json.Marshal(cached)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "update-check-info.json"), data, 0o600))

	release, err := checkForUpdate(t.Context(), nil)
	require.NoError(t, err)
	require.NotNil(t, release)
	assert.Equal(t, "99.0.0", release.Version)
}

func TestShouldNotifySuppressesRecentHomebrewRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake brew shell script requires a unix shell")
	}

	prefix := t.TempDir()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "brew"), []byte("#!/bin/sh\necho "+prefix+"\n"), 0o755))
	t.Setenv("PATH", binDir)

	recent := &update.ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now()}
	old := &update.ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now().Add(-48 * time.Hour)}
	homebrewBinary := filepath.Join(prefix, "bin", "shopware-cli")

	assert.False(t, shouldNotify(recent, homebrewBinary))
	assert.True(t, shouldNotify(recent, "/usr/local/other/shopware-cli"))
	assert.True(t, shouldNotify(old, homebrewBinary))
}

func TestIsUnderHomebrewFalseWhenBrewAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	assert.False(t, isUnderHomebrew("/usr/local/bin/shopware-cli"))
	assert.True(t, shouldNotify(&update.ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now()}, "/usr/local/bin/shopware-cli"))
}

func TestLookPathTreatsErrDotAsSuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relative PATH resolution semantics differ on windows")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "some-tool"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	t.Chdir(dir)
	t.Setenv("PATH", ".")

	resolved, err := lookPath("some-tool")
	require.NoError(t, err)
	assert.NotEmpty(t, resolved)

	_, err = lookPath("definitely-missing-tool")
	require.Error(t, err)
}
