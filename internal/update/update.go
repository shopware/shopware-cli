package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shyim/go-version"
)

// This regex matches git describe suffixes like "-123-gabcdef12".
var gitDescribeSuffixRE = regexp.MustCompile(`-\d+-g[a-f0-9]{7,40}$`)

const (
	updateCheckInterval = 24 * time.Hour
	githubAPITimeout    = 5 * time.Second
)

type ReleaseInfo struct {
	URL     string `json:"html_url"`
	Version string `json:"tag_name"`
}

type UpdateCheck struct {
	LastCheckedAt time.Time   `json:"last_checked_at"`
	LatestRelease ReleaseInfo `json:"latest_release"`
}

// CheckForUpdate checks whether an update exists for the Shopware CLI based on recency of last check within the given updateCheckInterval.
func CheckForUpdate(ctx context.Context, repo, buildVersion string) (*ReleaseInfo, error) {
	// Get last UpdateCheck from cache and return if it was checked within the given updateCheckInterval
	updateCheck, err := LoadUpdateCheckFromCache(ctx)
	if err != nil {
		return nil, err
	}
	if updateCheck != nil && time.Since(updateCheck.LastCheckedAt).Hours() < updateCheckInterval.Hours() {
		return nil, nil
	}

	// Fetch the latest release info from GitHub
	releaseInfo, err := getLatestReleaseInfo(ctx, &http.Client{Timeout: githubAPITimeout}, repo)
	if releaseInfo == nil || err != nil {
		return nil, err
	}

	// Save the new update check information to cache
	latestUpdateCheck := &UpdateCheck{
		LastCheckedAt: time.Now(),
		LatestRelease: *releaseInfo,
	}
	err = SaveUpdateCheckToCache(latestUpdateCheck)
	if err != nil {
		return nil, err
	}

	// Compare the latest release version with the current build version
	if versionGreaterThan(releaseInfo.Version, buildVersion) {
		return releaseInfo, nil
	}

	return nil, nil
}

// getLatestReleaseInfo fetches the latest release information from the GitHub API for the given repository.
func getLatestReleaseInfo(ctx context.Context, client *http.Client, repo string) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d", res.StatusCode)
	}
	dec := json.NewDecoder(res.Body)
	var latestRelease ReleaseInfo
	if err := dec.Decode(&latestRelease); err != nil {
		return nil, err
	}
	return &latestRelease, nil
}

// saveUpdateCheckToCache saves the update check information to a cache file.
func SaveUpdateCheckToCache(info *UpdateCheck) error {
	cacheFilePath := getUpdateCheckCacheFilePath()

	content, err := json.Marshal(info)
	if err != nil {
		return err
	}

	cacheDir := filepath.Dir(cacheFilePath)
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return err
	}

	err = os.WriteFile(cacheFilePath, content, 0o644)
	if err != nil {
		return err
	}

	return nil
}

// IsCI determines if the current execution context is within a known CI/CD system.
// This is based on https://github.com/watson/ci-info/blob/HEAD/index.js.
func IsCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}

// IsGitHubActions determines if the current execution context is within GitHub Actions.
// GitHub Actions sets the GITHUB_ACTIONS environment variable to "true" for all steps.
// See https://docs.github.com/en/actions/learn-github-actions/variables#default-environment-variables.
func IsGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}

// ShouldCheckForUpdate decides whether the CLI checks for updates based on user preferences and execution context.
func ShouldCheckForUpdate(version string) bool {
	// Todo: add option to disable update check

	if version == "dev" {
		return false
	}

	if IsCI() {
		return false
	}

	if IsGitHubActions() {
		return false
	}

	return true
}

func getUpdateCheckCacheFileName() string {
	return "update-check-info.json"
}

func getUpdateCheckCacheFilePath() string {
	return filepath.Join(system.GetShopwareCliCacheDir(), getUpdateCheckCacheFileName())
}

func LoadUpdateCheckFromCache(ctx context.Context) (*UpdateCheck, error) {
	cacheFilePath := getUpdateCheckCacheFilePath()

	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil, nil
	}

	content, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return nil, err
	}

	var info UpdateCheck
	err = json.Unmarshal(content, &info)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func versionGreaterThan(v, w string) bool {
	w = gitDescribeSuffixRE.ReplaceAllString(w, "")

	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}
