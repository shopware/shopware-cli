package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCheckForUpdate(t *testing.T) {
	scenarios := []struct {
		name           string
		currentVersion string
		latestVersion  string
		latestURL      string
		expectsResult  bool
	}{
		{
			name:           "latest is newer",
			currentVersion: "v0.0.1",
			latestVersion:  "v1.0.0",
			latestURL:      "https://example.com/release",
			expectsResult:  true,
		},
		{
			name:           "current is prerelease",
			currentVersion: "v1.0.0-rc.1",
			latestVersion:  "v1.0.0",
			latestURL:      "https://example.com/release",
			expectsResult:  true,
		},
		{
			name:           "current is built from source",
			currentVersion: "v1.2.3-123-gdeadbeef",
			latestVersion:  "v1.2.3",
			latestURL:      "https://example.com/release",
			expectsResult:  false,
		},
		{
			name:           "current is built from source after a prerelease",
			currentVersion: "v1.2.3-rc.1-123-gdeadbeef",
			latestVersion:  "v1.2.3",
			latestURL:      "https://example.com/release",
			expectsResult:  true,
		},
		{
			name:           "latest is newer than source build",
			currentVersion: "v1.2.3-123-gdeadbeef",
			latestVersion:  "v1.2.4",
			latestURL:      "https://example.com/release",
			expectsResult:  true,
		},
		{
			name:           "latest is current",
			currentVersion: "v1.0.0",
			latestVersion:  "v1.0.0",
			latestURL:      "https://example.com/release",
			expectsResult:  false,
		},
		{
			name:           "latest is older",
			currentVersion: "v0.10.0-rc.1",
			latestVersion:  "v0.9.0",
			latestURL:      "https://example.com/release",
			expectsResult:  false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

			requestCount := 0
			oldTransport := http.DefaultTransport
			http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
				requestCount++
				assert.Equal(t, "/repos/OWNER/REPO/releases/latest", req.URL.Path)

				payload := fmt.Sprintf(`{"tag_name":%q,"html_url":%q}`, s.latestVersion, s.latestURL)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(payload)),
					Request:    req,
				}, nil
			})
			t.Cleanup(func() {
				http.DefaultTransport = oldTransport
			})

			rel, err := CheckForUpdate(context.Background(), "OWNER/REPO", s.currentVersion)
			require.NoError(t, err)
			assert.Equal(t, 1, requestCount)

			if !s.expectsResult {
				assert.Nil(t, rel)
				return
			}

			require.NotNil(t, rel)
			assert.Equal(t, s.latestVersion, rel.Version)
			assert.Equal(t, s.latestURL, rel.URL)
		})
	}
}

func TestCheckForUpdateSkipsNetworkWhenCacheIsRecent(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	err := SaveUpdateCheckToCache(&UpdateCheck{
		LastCheckedAt: time.Now().Add(-(updateCheckInterval / 2)),
		LatestRelease: ReleaseInfo{Version: "v9.9.9", URL: "https://example.com/release"},
	})
	require.NoError(t, err)

	requestCount := 0
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		return nil, fmt.Errorf("unexpected network call")
	})
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})

	rel, checkErr := CheckForUpdate(context.Background(), "OWNER/REPO", "v1.0.0")
	require.NoError(t, checkErr)
	assert.Nil(t, rel)
	assert.Equal(t, 0, requestCount)
}

func TestSaveAndLoadUpdateCheckFromCache(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	expected := &UpdateCheck{
		LastCheckedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
		LatestRelease: ReleaseInfo{Version: "v1.2.3", URL: "https://example.com/release"},
	}

	err := SaveUpdateCheckToCache(expected)
	require.NoError(t, err)

	cacheFilePath := filepath.Join(os.Getenv("SHOPWARE_CLI_CACHE_DIR"), "update-check-info.json")
	_, statErr := os.Stat(cacheFilePath)
	require.NoError(t, statErr)

	actual, err := LoadUpdateCheckFromCache(context.Background())
	require.NoError(t, err)
	require.NotNil(t, actual)
	assert.Equal(t, expected.LatestRelease, actual.LatestRelease)
	assert.True(t, expected.LastCheckedAt.Equal(actual.LastCheckedAt))
}

func TestLoadUpdateCheckFromCacheWhenMissing(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	actual, err := LoadUpdateCheckFromCache(context.Background())
	require.NoError(t, err)
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
			t.Setenv("CI", "")
			t.Setenv("BUILD_NUMBER", "")
			t.Setenv("RUN_ID", "")
			t.Setenv("GITHUB_ACTIONS", "")

			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			assert.Equal(t, tt.expected, ShouldCheckForUpdate(tt.version))
		})
	}
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
	oldTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestCount++
		payload := `{"tag_name":"v9.9.9","html_url":"https://example.com/release"}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(payload)),
			Request:    req,
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = oldTransport
	})

	first, err := CheckForUpdate(context.Background(), "OWNER/REPO", "v1.0.0")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "v9.9.9", first.Version)

	second, err := CheckForUpdate(context.Background(), "OWNER/REPO", "v1.0.0")
	require.NoError(t, err)
	assert.Nil(t, second)

	err = SaveUpdateCheckToCache(&UpdateCheck{
		LastCheckedAt: time.Now().Add(-(updateCheckInterval + time.Second)),
		LatestRelease: ReleaseInfo{Version: "v9.9.9", URL: "https://example.com/release"},
	})
	require.NoError(t, err)

	third, err := CheckForUpdate(context.Background(), "OWNER/REPO", "v1.0.0")
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, "v9.9.9", third.Version)

	assert.Equal(t, 2, requestCount)
}
