package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

// composerPharURL points at the latest stable Composer PHAR; a var so tests
// can redirect it to a local server. A matching "<url>.sha256sum" must exist.
var composerPharURL = "https://getcomposer.org/download/latest-stable/composer.phar"

// ResolveComposer returns a usable Composer executable, preferring composer
// from PATH. When none is installed, it downloads the Composer PHAR into the
// shopware-cli cache directory (once) and returns its path. isPhar reports
// that the returned path must be run through a PHP binary instead of directly.
func ResolveComposer(ctx context.Context) (path string, isPhar bool, err error) {
	if composerBinary, lookErr := exec.LookPath("composer"); lookErr == nil {
		return composerBinary, false, nil
	}

	pharPath := filepath.Join(GetShopwareCliCacheDir(), "composer.phar")
	if _, statErr := os.Stat(pharPath); statErr == nil {
		return pharPath, true, nil
	}

	if err := downloadComposerPhar(ctx, pharPath); err != nil {
		return "", false, err
	}

	return pharPath, true, nil
}

func downloadComposerPhar(ctx context.Context, target string) error {
	logging.FromContext(ctx).Infof("Composer is not installed, downloading it from %s", composerPharURL)

	expectedSum, err := fetchComposerChecksum(ctx)
	if err != nil {
		return err
	}

	body, err := httpGet(ctx, composerPharURL)
	if err != nil {
		return err
	}
	defer func() {
		if err := body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close composer download body: %v", err)
		}
	}()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	// Download to a temp file and rename so a concurrent or aborted run never
	// sees a half-written PHAR at the final path.
	tmpFile, err := os.CreateTemp(filepath.Dir(target), "composer-*.phar.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(tmpFile, hash), body)
	closeErr := tmpFile.Close()
	if copyErr != nil {
		return fmt.Errorf("cannot download composer: %w", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}

	if actualSum := hex.EncodeToString(hash.Sum(nil)); actualSum != expectedSum {
		return fmt.Errorf("composer download is corrupted: checksum %s does not match expected %s", actualSum, expectedSum)
	}

	if err := os.Chmod(tmpFile.Name(), 0o755); err != nil {
		return err
	}

	return os.Rename(tmpFile.Name(), target)
}

// fetchComposerChecksum returns the expected SHA-256 of the Composer PHAR from
// the published "<url>.sha256sum" file (format: "<hex>  composer.phar").
func fetchComposerChecksum(ctx context.Context) (string, error) {
	body, err := httpGet(ctx, composerPharURL+".sha256sum")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close composer checksum body: %v", err)
		}
	}()

	content, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("cannot read composer checksum: %w", err)
	}

	sum, _, _ := strings.Cut(strings.TrimSpace(string(content)), " ")
	if sum == "" {
		return "", fmt.Errorf("composer checksum file %s.sha256sum is empty", composerPharURL)
	}

	return sum, nil
}

func httpGet(ctx context.Context, url string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("cannot download %s: %w", url, err)
	}

	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("cannot download %s: got %s", url, resp.Status)
	}

	return resp.Body, nil
}
