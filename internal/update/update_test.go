package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newVersionResponseClient(latestVersion string, requestCount *int) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			*requestCount++

			body := fmt.Sprintf(`{"version":"%s"}`, latestVersion)

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}

func TestCheckForUpdate(t *testing.T) {
	scenarios := []struct {
		name           string
		currentVersion string
		latestVersion  string
		expectsResult  bool
	}{
		{
			name:           "latest is newer",
			currentVersion: "v0.0.1",
			latestVersion:  "v1.0.0",
			expectsResult:  true,
		},
		{
			name:           "current is prerelease",
			currentVersion: "v1.0.0-rc.1",
			latestVersion:  "v1.0.0",
			expectsResult:  true,
		},
		{
			name:           "current is built from source",
			currentVersion: "v1.2.3-123-gdeadbeef",
			latestVersion:  "v1.2.3",
			expectsResult:  false,
		},
		{
			name:           "current is built from source after a prerelease",
			currentVersion: "v1.2.3-rc.1-123-gdeadbeef",
			latestVersion:  "v1.2.3",
			expectsResult:  true,
		},
		{
			name:           "latest is newer than source build",
			currentVersion: "v1.2.3-123-gdeadbeef",
			latestVersion:  "v1.2.4",
			expectsResult:  true,
		},
		{
			name:           "latest is current",
			currentVersion: "v1.0.0",
			latestVersion:  "v1.0.0",
			expectsResult:  false,
		},
		{
			name:           "latest is older",
			currentVersion: "v0.10.0-rc.1",
			latestVersion:  "v0.9.0",
			expectsResult:  false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

			requestCount := 0
			client := newVersionResponseClient(s.latestVersion, &requestCount)

			rel, err := CheckForUpdate(t.Context(), s.currentVersion, client)
			assert.Equal(t, 1, requestCount)

			if !s.expectsResult {
				require.ErrorIs(t, err, ErrNoUpdateAvailable)
				assert.Nil(t, rel)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, rel)
			assert.Equal(t, s.latestVersion, rel.Version)
		})
	}
}

func TestCheckForUpdateSkipsNetworkWhenCacheIsRecent(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	err := SaveReleaseInfoToCache(&ReleaseInfo{
		Version:   "v9.9.9",
		FetchedAt: time.Now().Add(-(updateCheckInterval / 2)),
	})
	require.NoError(t, err)

	requestCount := 0
	client := newVersionResponseClient("v9.9.9", &requestCount)

	rel, checkErr := CheckForUpdate(t.Context(), "v1.0.0", client)
	require.ErrorIs(t, checkErr, ErrNoUpdateAvailable)
	assert.Nil(t, rel)
	assert.Equal(t, 0, requestCount)
}

func TestSaveAndLoadUpdateCheckFromCache(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	expected := &ReleaseInfo{
		Version:     "v1.2.3",
		PublishedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
		FetchedAt:   time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC),
	}

	err := SaveReleaseInfoToCache(expected)
	require.NoError(t, err)

	cacheFilePath := filepath.Join(os.Getenv("SHOPWARE_CLI_CACHE_DIR"), "update-check-info.json")
	_, statErr := os.Stat(cacheFilePath)
	require.NoError(t, statErr)

	actual, err := LoadReleaseInfoFromCache()
	require.NoError(t, err)
	require.NotNil(t, actual)
	assert.Equal(t, expected.Version, actual.Version)
	assert.True(t, expected.PublishedAt.Equal(actual.PublishedAt))
	assert.True(t, expected.FetchedAt.Equal(actual.FetchedAt))
}

func TestLoadUpdateCheckFromCacheWhenMissing(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	actual, err := LoadReleaseInfoFromCache()
	require.ErrorIs(t, err, ErrNoCacheFile)
	assert.Nil(t, actual)
}

func TestShouldCheckForUpdate(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		env      map[string]string
		expected bool
	}{
		{
			name:    "disabled via legacy env var",
			version: "v1.0.0",
			env: map[string]string{
				"SHOPWARE_CLI_NO_UPDATE_NOTIFICATION": "1",
			},
			expected: false,
		},
		{
			name:    "disabled via true env value",
			version: "v1.0.0",
			env: map[string]string{
				"SHOPWARE_CLI_NO_UPDATE_NOTIFICATION": "true",
			},
			expected: false,
		},
		{
			name:     "disabled on dev version",
			version:  "dev",
			expected: false,
		},
		{
			name:    "disabled in generic ci",
			version: "v1.0.0",
			env: map[string]string{
				"CI": "1",
			},
			expected: false,
		},
		{
			name:    "disabled in build-number ci",
			version: "v1.0.0",
			env: map[string]string{
				"BUILD_NUMBER": "123",
			},
			expected: false,
		},
		{
			name:    "disabled in run-id ci",
			version: "v1.0.0",
			env: map[string]string{
				"RUN_ID": "123",
			},
			expected: false,
		},
		{
			name:    "disabled in github actions",
			version: "v1.0.0",
			env: map[string]string{
				"GITHUB_ACTIONS": "true",
			},
			expected: false,
		},
		{
			name:     "enabled on regular local run",
			version:  "v1.0.0",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
			t.Setenv("CI", "")
			t.Setenv("BUILD_NUMBER", "")
			t.Setenv("RUN_ID", "")
			t.Setenv("GITHUB_ACTIONS", "")
			t.Setenv("SHOPWARE_CLI_NO_UPDATE_NOTIFICATION", "")
			t.Setenv("SHOPWARE_CLI_DISABLE_VERSION_CHECK", "")

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			assert.Equal(t, tt.expected, ShouldCheckForUpdate(tt.version, []string{}))
		})
	}
}

func TestInstallationContextDetectsHomebrew(t *testing.T) {
	workspace := t.TempDir()
	brewPrefix := filepath.Join(workspace, "homebrew")
	brewBinDir := filepath.Join(brewPrefix, "bin")
	require.NoError(t, os.MkdirAll(brewBinDir, 0o755))

	brewScriptDir := filepath.Join(workspace, "tools")
	require.NoError(t, os.MkdirAll(brewScriptDir, 0o755))
	brewScriptPath := filepath.Join(brewScriptDir, "brew")
	require.NoError(t, os.WriteFile(brewScriptPath, []byte("#!/bin/sh\nprintf '%s\\n' \""+brewPrefix+"\"\n"), 0o755))

	t.Setenv("PATH", brewScriptDir)

	assert.Equal(t, "brew", InstallationContext(filepath.Join(brewBinDir, "shopware-cli")))
}

func TestInstallationContextDetectsApt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("apt detection only applies on Linux")
	}

	workspace := t.TempDir()
	aptScriptDir := filepath.Join(workspace, "tools")
	require.NoError(t, os.MkdirAll(aptScriptDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(aptScriptDir, "apt"), []byte("#!/bin/sh\nexit 0\n"), 0o755))

	t.Setenv("PATH", aptScriptDir)

	assert.Equal(t, "apt", InstallationContext(filepath.Join(workspace, "bin", "shopware-cli")))
}

func TestVersionGreaterThan(t *testing.T) {
	tests := []struct {
		name     string
		latest   string
		current  string
		expected bool
	}{
		{
			name:     "newer release",
			latest:   "v1.0.0",
			current:  "v0.9.0",
			expected: true,
		},
		{
			name:     "same release",
			latest:   "v1.0.0",
			current:  "v1.0.0",
			expected: false,
		},
		{
			name:     "older release",
			latest:   "v0.9.0",
			current:  "v1.0.0",
			expected: false,
		},
		{
			name:     "source build treated as ahead of release",
			latest:   "v1.2.3",
			current:  "v1.2.3-123-gdeadbeef",
			expected: false,
		},
		{
			name:     "source build after prerelease still needs stable",
			latest:   "v1.2.3",
			current:  "v1.2.3-rc.1-123-gdeadbeef",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, versionGreaterThan(tt.latest, tt.current))
		})
	}
}

func TestUpdateHintRespectsConfiguredInterval(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	requestCount := 0
	client := newVersionResponseClient("v9.9.9", &requestCount)

	first, err := CheckForUpdate(t.Context(), "v0.1.0", client)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "v9.9.9", first.Version)

	second, err := CheckForUpdate(t.Context(), "v0.1.0", client)
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
	assert.Nil(t, second)

	err = SaveReleaseInfoToCache(&ReleaseInfo{
		Version:   "v9.9.9",
		FetchedAt: time.Now().Add(-(updateCheckInterval + time.Second)),
	})
	require.NoError(t, err)

	third, err := CheckForUpdate(t.Context(), "v0.1.0", client)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, "v9.9.9", third.Version)

	assert.Equal(t, 2, requestCount)
}
