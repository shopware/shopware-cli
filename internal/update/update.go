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
	// releaseFetchInterval is the minimum duration between update checks.
	releaseFetchInterval = 24 * time.Hour
	notificationInterval = 24 * time.Hour

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

type UpdateNotification struct {
	LastPrintedAt time.Time `json:"last_printed_at"`
	LastVersion   string    `json:"last_version"`
}

// IsRecent checks if the release was published within the last 24 hours.
func (r ReleaseInfo) IsRecent() bool {
	return !r.PublishedAt.IsZero() && time.Since(r.PublishedAt) < 24*time.Hour
}

func (r ReleaseInfo) IsFetchedWithin(interval time.Duration) bool {
	return !r.FetchedAt.IsZero() && time.Since(r.FetchedAt) < interval
}

// CheckForUpdate checks if a newer version is available, if the last check is more than 24 hours ago, and returns the fetched release information if so.
func CheckForUpdate(ctx context.Context, buildVersion string, client *http.Client) (*ReleaseInfo, error) {
	latestReleaseInfo, err := getReleaseInformation(ctx, client)
	if err != nil {
		return nil, err
	}

	// Compare latest version with the build version; return the release info if the installed version is older.
	if versionGreaterThan(latestReleaseInfo.Version, buildVersion) {
		return latestReleaseInfo, nil
	}

	return nil, ErrNoUpdateAvailable
}

// ShouldPrintUpdateHint reports whether the CLI notification for version may
// be printed. The TUI hint is intentionally not governed by this function.
func ShouldPrintUpdateHint(version string) bool {
	notification, err := loadUpdateNotificationFromCache()
	if err != nil && !errors.Is(err, ErrNoCacheFile) {
		return false
	}

	if notification != nil &&
		!notification.LastPrintedAt.IsZero() &&
		time.Since(notification.LastPrintedAt) < notificationInterval &&
		(notification.LastVersion == "" || notification.LastVersion == version) {
		return false
	}

	return true
}

// MarkUpdateNotificationPrinted records that the detailed CLI notification
// was actually shown to the user.
func MarkUpdateNotificationPrinted(version string) error {
	return saveUpdateNotificationToCache(&UpdateNotification{
		LastPrintedAt: time.Now(),
		LastVersion:   version,
	})
}

func getReleaseInformation(ctx context.Context, client *http.Client) (*ReleaseInfo, error) {
	// Load cached release info
	cachedReleaseInfo, err := loadReleaseInfoFromCache()
	if err != nil && !errors.Is(err, ErrNoCacheFile) {
		return nil, err
	}

	// fetch latest release info if no cached release info is available or if the cached release info is not recent.
	if errors.Is(err, ErrNoCacheFile) || !cachedReleaseInfo.IsFetchedWithin(releaseFetchInterval) {
		if client == nil {
			client = &http.Client{Timeout: 5 * time.Second}
		}

		fetchedReleaseInfo, err := fetchLatestReleaseInfoFromGitHubPages(ctx, client)
		if err != nil {
			return nil, err
		}

		// Save timestamp + fetched release info to cache.
		fetchedReleaseInfo.FetchedAt = time.Now()
		err = saveReleaseInfoToCache(fetchedReleaseInfo)
		if err != nil {
			return nil, err
		}

		return fetchedReleaseInfo, nil
	}

	return cachedReleaseInfo, nil
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

func loadReleaseInfoFromCache() (*ReleaseInfo, error) {
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

func saveReleaseInfoToCache(info *ReleaseInfo) error {
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

func getUpdateNotificationCacheFilePath() string {
	return filepath.Join(system.GetShopwareCliCacheDir(), "update-notification.json")
}

func loadUpdateNotificationFromCache() (*UpdateNotification, error) {
	cacheFilePath := getUpdateNotificationCacheFilePath()

	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil, ErrNoCacheFile
	}

	content, err := os.ReadFile(cacheFilePath)
	if err != nil {
		return nil, err
	}

	var notification UpdateNotification
	err = json.Unmarshal(content, &notification)
	if err != nil {
		return nil, err
	}

	return &notification, nil
}

func saveUpdateNotificationToCache(notification *UpdateNotification) error {
	cacheFilePath := getUpdateNotificationCacheFilePath()

	content, err := json.Marshal(notification)
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
