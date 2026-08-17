package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
)

const (
	// updateCheckInterval is the minimum duration between update checks.
	updateCheckInterval = 24 * time.Hour

	// This is the primary source of truth for the latest release information, as it is maintained and updated with each new release.
	latestReleaseURL = "https://shopware.github.io/shopware-cli/version.json"
	repositoryURL    = "https://github.com/shopware/shopware-cli"

	// noUpdateNotificationEnv can be set to "true" by the user to disable update notifications.
	noUpdateNotificationEnv = "SHOPWARE_CLI_NO_UPDATE_NOTIFICATION"
)

// This regex matches git describe suffixes like "-123-gabcdef12".
var gitDescribeSuffixRE = regexp.MustCompile(`-\d+-g[a-f0-9]{7,40}$`)

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
	// Load cached release info; continue if no cache file exists yet.
	cachedReleaseInfo, err := LoadReleaseInfoFromCache()
	if err != nil && !errors.Is(err, ErrNoCacheFile) {
		return nil, err
	}
	// Early return if cached release info was fetched within the given releaseFetchInterval.
	if cachedReleaseInfo != nil {
		if time.Since(cachedReleaseInfo.FetchedAt) < updateCheckInterval {
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
	linkStyle := lipgloss.NewStyle().Foreground(tui.TextColor).Underline(true)

	firstLine := strings.Join([]string{
		warnBoldStyle.Render("⁺₊⋆"),
		boldStyle.Render("Update available!"),
		boldStyle.Render(buildVersion),
		boldStyle.Render("→"),
		boldStyle.Render(latestVersion),
		warnBoldStyle.Render("⋆₊⁺"),
	}, " ")

	updateInstruction := "Visit " + tui.StyledLink(repositoryURL, repositoryURL, linkStyle) + " to view installation options."
	secondLine := lipgloss.NewStyle().Foreground(tui.TextColor).Render(updateInstruction)

	notificationContent := firstLine + "\n" + secondLine

	renderedNotificationContent := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(tui.BorderColor).
		Padding(0, 1).
		Render(notificationContent)

	return renderedNotificationContent
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

	if os.Getenv(noUpdateNotificationEnv) == "true" {
		return false
	}

	if version == "dev" {
		return false
	}

	if system.IsCIEnvironment(os.Getenv) {
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

// fetchLatestReleaseInfoFromGitHubPages fetches the latest release information from the version.json file hosted on GitHub Pages.
func fetchLatestReleaseInfoFromGitHubPages(ctx context.Context, client *http.Client) (releaseInfo *ReleaseInfo, err error) {
	req, err := http.NewRequestWithContext(ctx, "GET", latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := res.Body.Close(); err == nil && closeErr != nil {
			releaseInfo = nil
			err = closeErr
		}
	}()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d", res.StatusCode)
	}

	var latestRelease ReleaseInfo

	dec := json.NewDecoder(res.Body)
	err = dec.Decode(&latestRelease)
	if err != nil {
		return nil, err
	}

	latestRelease.FetchedAt = time.Now()

	return &latestRelease, nil
}

// versionGreaterThan compares two semantic version strings and returns true if v is greater than w.
func versionGreaterThan(v, w string) bool {
	w = gitDescribeSuffixRE.ReplaceAllString(w, "")

	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}

func getUpdateCheckCacheFilePath() string {
	return filepath.Join(system.GetShopwareCliCacheDir(), "update-check-info.json")
}
