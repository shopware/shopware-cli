package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

// This regex matches git describe suffixes like "-123-gabcdef12".
var gitDescribeSuffixRE = regexp.MustCompile(`-\d+-g[a-f0-9]{7,40}$`)

const (
	updateCheckInterval = 24 * time.Hour

	// This is the primary source of truth for the latest release information, as it is maintained and updated with each new release.
	latestReleaseURL        = "https://shopware.github.io/shopware-cli/version.json"
	noUpdateNotificationEnv = "SHOPWARE_CLI_NO_UPDATE_NOTIFICATION"
)

var ErrNoUpdateAvailable = errors.New("no update available")
var ErrNoCacheFile = errors.New("no update cache file")

type ReleaseInfo struct {
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"published_at"`
	FetchedAt   time.Time `json:"fetched_at"`
}

// IsRecent checks if the release was published within the last 24 hours.
func (r ReleaseInfo) IsRecent() bool {
	return !r.PublishedAt.IsZero() && time.Since(r.PublishedAt) < 24*time.Hour
}

// CheckForUpdate checks if a newer version is available, if the last check is more than 24 hours ago, and returns the fetched release information if so.
func CheckForUpdate(ctx context.Context, buildVersion string, client *http.Client) (*ReleaseInfo, error) {
	// Load cached release info.
	cachedReleaseInfo, err := LoadReleaseInfoFromCache()
	if err != nil && !errors.Is(err, ErrNoCacheFile) {
		return nil, err
	}
	// Early return if latest release info was fetched within the given releaseFetchInterval.
	if cachedReleaseInfo != nil {
		lastCheck := cachedReleaseInfo.FetchedAt
		if lastCheck.IsZero() {
			// Backward compatibility for old cache entries.
			lastCheck = cachedReleaseInfo.PublishedAt
		}

		if !lastCheck.IsZero() && time.Since(lastCheck) < updateCheckInterval {
			return nil, ErrNoUpdateAvailable
		}
	}

	// Fetch latest release info.
	latestReleaseInfo, err := fetchLatestReleaseInfoFromGitHubPages(ctx, client)
	if latestReleaseInfo == nil || err != nil {
		return nil, err
	}

	// Save timestamp + fetched release info to cache.
	latestReleaseInfo.FetchedAt = time.Now()
	err = SaveReleaseInfoToCache(latestReleaseInfo)
	if err != nil {
		return nil, err
	}

	// Compare latest version with the build version; return the release info if the installed version is older.
	if versionGreaterThan(latestReleaseInfo.Version, buildVersion) {
		return latestReleaseInfo, nil
	}

	return nil, ErrNoUpdateAvailable
}

func RenderUpdateNotification(latestVersion string, buildVersion string) string {
	warnBoldStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.WarnColor)
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.TextColor)

	firstLine := strings.Join([]string{
		warnBoldStyle.Render("⁺₊⋆"),
		boldStyle.Render("Update available!"),
		boldStyle.Render(buildVersion),
		boldStyle.Render("→"),
		boldStyle.Render(latestVersion),
		warnBoldStyle.Render("⋆₊⁺"),
	}, " ")

	secondLine := lipgloss.NewStyle().Foreground(tui.TextColor).Render(getUpdateMethod())
	notificationContent := firstLine + "\n " + secondLine

	renderedUpdateNotification := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(tui.BorderColor).
		Padding(0, 1).
		Render(notificationContent)

	return renderedUpdateNotification
}

// ShouldCheckForUpdate decides whether the CLI checks for updates based on user preferences and execution context.
func ShouldCheckForUpdate(version string, args []string) bool {
	if len(args) > 0 {
		for _, arg := range args {
			if arg == "--no-update-hint" || arg == "-n" {
				return false
			}
		}
	}

	if os.Getenv(noUpdateNotificationEnv) == "1" || os.Getenv(noUpdateNotificationEnv) == "true" {
		return false
	}

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

func LoadReleaseInfoFromCache() (*ReleaseInfo, error) {
	cacheFilePath := getUpdateCheckCacheFilePath()

	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil, ErrNoCacheFile
	}

	content, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return nil, err
	}

	var info ReleaseInfo
	err = json.Unmarshal(content, &info)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func SaveReleaseInfoToCache(info *ReleaseInfo) error {
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

func getUpdateMethod() string {
	binaryPath, err := os.Executable()
	if err != nil {
		return "Download the latest version from https://github.com/shopware/shopware-cli/releases"
	}

	switch InstallationContext(binaryPath) {
	case "brew":
		return "Update via `brew update && brew upgrade shopware-cli`"
	case "apt":
		return "Update via `sudo apt update && sudo apt upgrade shopware-cli`"
	default:
		return "Download the latest version from https://github.com/shopware/shopware-cli/releases"
	}
}

// fetchLatestReleaseInfoFromGitHubPages fetches the latest release information from the version.json file hosted on GitHub Pages.
func fetchLatestReleaseInfoFromGitHubPages(ctx context.Context, client *http.Client) (*ReleaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d", res.StatusCode)
	}
	dec := json.NewDecoder(res.Body)

	var latestRelease ReleaseInfo
	if err := dec.Decode(&latestRelease); err != nil {
		return nil, err
	}

	latestRelease.FetchedAt = time.Now()

	return &latestRelease, nil
}

func versionGreaterThan(v, w string) bool {
	w = gitDescribeSuffixRE.ReplaceAllString(w, "")

	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}

func getUpdateCheckCacheFilePath() string {
	return filepath.Join(system.GetShopwareCliCacheDir(), "update-check-info.json")
}

// InstallationContext reports the install/update channel for a binary path.
func InstallationContext(binaryPath string) string {
	if binaryPath != "" && IsUnderHomebrew(binaryPath) {
		return "brew"
	}

	if runtime.GOOS == "linux" && isUnderApt() {
		return "apt"
	}

	return "other"
}

// IsUnderHomebrew reports whether the binary resides in the active Homebrew prefix.
func IsUnderHomebrew(binaryPath string) bool {
	brewExe, err := lookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.CommandContext(context.Background(), brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)

	return strings.HasPrefix(binaryPath, brewBinPrefix)
}

func isUnderApt() bool {
	if _, err := os.Stat("/etc/debian_version"); err == nil {
		return true
	}

	_, err := lookPath("apt")
	return err == nil
}

// lookPath allows safe execution of the LookPath function, handling the ErrDot case.
func lookPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if errors.Is(err, exec.ErrDot) {
		return path, nil
	}
	return path, err
}
