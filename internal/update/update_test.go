package update

import (
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

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newVersionResponseClient(latestVersion string, requestCount *int) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			(*requestCount)++

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

	err := saveReleaseInfoToCache(&ReleaseInfo{
		Version:   "v9.9.9",
		FetchedAt: time.Now().Add(-(releaseFetchInterval / 2)),
	})
	require.NoError(t, err)

	requestCount := 0
	client := newVersionResponseClient("v9.9.9", &requestCount)

	rel, checkErr := CheckForUpdate(t.Context(), "v1.0.0", client)
	require.NoError(t, checkErr)
	assert.Equal(t, "v9.9.9", rel.Version)
	assert.Equal(t, 0, requestCount)
}

func TestSaveAndLoadUpdateCheckFromCache(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	expected := &ReleaseInfo{
		Version:     "v1.2.3",
		PublishedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
		FetchedAt:   time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC),
	}

	err := saveReleaseInfoToCache(expected)
	require.NoError(t, err)

	cacheFilePath := filepath.Join(os.Getenv("SHOPWARE_CLI_CACHE_DIR"), "update-check-info.json")
	_, statErr := os.Stat(cacheFilePath)
	require.NoError(t, statErr)

	actual, err := loadReleaseInfoFromCache()
	require.NoError(t, err)
	require.NotNil(t, actual)
	assert.Equal(t, expected.Version, actual.Version)
	assert.True(t, expected.PublishedAt.Equal(actual.PublishedAt))
	assert.True(t, expected.FetchedAt.Equal(actual.FetchedAt))
}

func TestLoadUpdateCheckFromCacheWhenMissing(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	actual, err := loadReleaseInfoFromCache()
	require.ErrorIs(t, err, ErrNoCacheFile)
	assert.Nil(t, actual)
}

func TestRenderUpdateNotificationContainsReleaseInformation(t *testing.T) {
	rendered := RenderUpdateNotification("v2.0.0", "v1.0.0")

	assert.Contains(t, rendered, "Update available!")
	assert.Contains(t, rendered, "v1.0.0")
	assert.Contains(t, rendered, "v2.0.0")
	assert.Contains(t, rendered, repositoryURL)
}

func TestShouldCheckForUpdate(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		args     []string
		env      map[string]string
		expected bool
	}{
		{
			name:    "unsupported environment value does not disable notifications",
			version: "v1.0.0",
			env: map[string]string{
				"SHOPWARE_CLI_NO_UPDATE_NOTIFICATION": "1",
			},
			expected: true,
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
				"CI": "true",
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
			name:     "disabled via long command-line flag",
			version:  "v1.0.0",
			args:     []string{"--no-update-hint"},
			expected: false,
		},
		{
			name:     "disabled via short command-line flag",
			version:  "v1.0.0",
			args:     []string{"-n"},
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
			t.Setenv("GITHUB_ACTIONS", "")
			t.Setenv("GITLAB_CI", "")
			t.Setenv("JENKINS_URL", "")
			t.Setenv("BUILDKITE", "")
			t.Setenv("CIRCLECI", "")
			t.Setenv("DRONE", "")
			t.Setenv("TEAMCITY_VERSION", "")
			t.Setenv("TF_BUILD", "")
			t.Setenv("SHOPWARE_CLI_NO_UPDATE_NOTIFICATION", "")
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			assert.Equal(t, tt.expected, ShouldCheckForUpdate(tt.version, tt.args))
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

func TestCheckForUpdateHonorsCacheInterval(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	requestCount := 0
	client := newVersionResponseClient("v9.9.9", &requestCount)

	first, err := CheckForUpdate(t.Context(), "v0.1.0", client)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "v9.9.9", first.Version)

	second, err := CheckForUpdate(t.Context(), "v0.1.0", client)
	require.NoError(t, err)
	assert.Equal(t, "v9.9.9", second.Version)
	assert.Equal(t, 1, requestCount)

	err = saveReleaseInfoToCache(&ReleaseInfo{
		Version:   "v9.9.9",
		FetchedAt: time.Now().Add(-(releaseFetchInterval + time.Second)),
	})
	require.NoError(t, err)

	third, err := CheckForUpdate(t.Context(), "v0.1.0", client)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, "v9.9.9", third.Version)

	assert.Equal(t, 2, requestCount)
}

func TestUpdateNotificationIntervalIsPerVersion(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	assert.True(t, ShouldPrintUpdateHint("v2.0.0"))
	require.NoError(t, MarkUpdateNotificationPrinted("v2.0.0"))
	assert.False(t, ShouldPrintUpdateHint("v2.0.0"))
	assert.True(t, ShouldPrintUpdateHint("v3.0.0"))
}
