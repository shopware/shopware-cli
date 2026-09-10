package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/update"
)

// isolateUpdateEnvironment points caches at a temp dir, hides brew from PATH
// so the binary never counts as Homebrew-managed, and clears CI markers.
func isolateUpdateEnvironment(t *testing.T) string {
	t.Helper()

	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHOPWARE_CLI_NO_UPDATE_NOTIFICATION", "")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")

	return cacheDir
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	original := version
	version = v
	t.Cleanup(func() { version = original })
}

func restoreUpdateAvailable(t *testing.T) {
	t.Helper()
	original := tui.UpdateAvailable
	t.Cleanup(func() { tui.UpdateAvailable = original })
}

func TestStartUpdateCheck_DevBuildReportsNoRelease(t *testing.T) {
	isolateUpdateEnvironment(t)
	setVersion(t, "dev")
	restoreUpdateAvailable(t)

	handle, cancel := startUpdateCheck(context.Background(), nil)
	defer cancel()

	result := handle.Wait(context.Background())
	assert.Nil(t, result.Release)
	assert.ErrorIs(t, result.Err, update.ErrNoUpdateAvailable)

	require.NotNil(t, tui.UpdateAvailable)
	assert.False(t, tui.UpdateAvailable(context.Background()))
}

func TestStartUpdateCheck_OptOutFlagSkipsCheck(t *testing.T) {
	isolateUpdateEnvironment(t)
	setVersion(t, "1.0.0")
	restoreUpdateAvailable(t)

	for _, flag := range []string{"--no-update-hint", "-n"} {
		t.Run(flag, func(t *testing.T) {
			handle, cancel := startUpdateCheck(context.Background(), []string{"project", "console", flag})
			defer cancel()

			result := handle.Wait(context.Background())
			assert.Nil(t, result.Release)
			assert.ErrorIs(t, result.Err, update.ErrNoUpdateAvailable)
			assert.False(t, tui.UpdateAvailable(context.Background()))
		})
	}
}

func TestPrintUpdateHint_NoReleaseDoesNothing(t *testing.T) {
	cacheDir := isolateUpdateEnvironment(t)

	var out bytes.Buffer
	printUpdateHint(context.Background(), &out, nil)

	assert.Empty(t, out.String())
	assert.NoFileExists(t, filepath.Join(cacheDir, "update-notification.json"))
}

func TestPrintUpdateHint_PrintsOnceAndRecordsTimestamp(t *testing.T) {
	cacheDir := isolateUpdateEnvironment(t)
	setVersion(t, "1.0.0")

	release := &update.ReleaseInfo{Version: "2.0.0", PublishedAt: time.Now().Add(-48 * time.Hour)}

	var first bytes.Buffer
	printUpdateHint(context.Background(), &first, release)

	assert.Contains(t, first.String(), "Update available!")
	assert.Contains(t, first.String(), "1.0.0")
	assert.Contains(t, first.String(), "2.0.0")
	assert.FileExists(t, filepath.Join(cacheDir, "update-notification.json"))

	var second bytes.Buffer
	printUpdateHint(context.Background(), &second, release)

	assert.Empty(t, second.String(), "hint must not be repeated within the notification interval")
}

func TestPrintUpdateHint_RespectsRecentNotification(t *testing.T) {
	cacheDir := isolateUpdateEnvironment(t)
	setVersion(t, "1.0.0")

	require.NoError(t, os.WriteFile(
		filepath.Join(cacheDir, "update-notification.json"),
		[]byte(`{"last_printed_at":"`+time.Now().Add(-time.Hour).Format(time.RFC3339)+`"}`),
		0o644,
	))

	var out bytes.Buffer
	printUpdateHint(context.Background(), &out, &update.ReleaseInfo{Version: "2.0.0"})

	assert.Empty(t, out.String())
}
