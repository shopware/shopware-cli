package system

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	composerInstallerURL          = "https://getcomposer.org/installer"
	composerInstallerSignatureURL = "https://composer.github.io/installer.sig"
)

type Composer struct {
	Path      string
	PHPBinary string
	Temporary bool
	cleanup   func()
}

func (c *Composer) Cleanup() {
	if c.cleanup != nil {
		c.cleanup()
		c.cleanup = nil
	}
}

func (c *Composer) Command(ctx context.Context, args ...string) *exec.Cmd {
	if c.PHPBinary != "" {
		return exec.CommandContext(ctx, c.PHPBinary, append([]string{c.Path}, args...)...)
	}

	return exec.CommandContext(ctx, c.Path, args...)
}

type ComposerResolver struct {
	HTTPClient            *http.Client
	InstallerURL          string
	InstallerSignatureURL string
	LookPath              func(string) (string, error)
	ResolvePHPBinary      func() (string, error)
	RunInstaller          func(context.Context, string, string, string) error
	TempDir               string
}

func NewComposerResolver() *ComposerResolver {
	return &ComposerResolver{
		HTTPClient:            &http.Client{Timeout: 30 * time.Second},
		InstallerURL:          composerInstallerURL,
		InstallerSignatureURL: composerInstallerSignatureURL,
		LookPath:              exec.LookPath,
		ResolvePHPBinary:      resolvePHPBinary,
		RunInstaller:          runComposerInstaller,
	}
}

func (r *ComposerResolver) Resolve(ctx context.Context) (*Composer, error) {
	if composerPath, err := r.LookPath("composer"); err == nil {
		return &Composer{Path: composerPath, PHPBinary: os.Getenv("PHP_BINARY")}, nil
	}

	phpBinary, err := r.ResolvePHPBinary()
	if err != nil {
		return nil, fmt.Errorf("composer was not found and a PHP binary could not be resolved: %w", err)
	}

	tempDir, err := os.MkdirTemp(r.TempDir, "shopware-cli-composer-*")
	if err != nil {
		return nil, fmt.Errorf("composer was not found and a temporary Composer download could not be prepared: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tempDir) }

	installerPath := filepath.Join(tempDir, "composer-setup.php")
	signature, err := r.downloadInstaller(ctx, installerPath)
	if err != nil {
		cleanup()
		return nil, composerDownloadError(err)
	}

	if err := verifyInstaller(installerPath, signature); err != nil {
		cleanup()
		return nil, composerDownloadError(err)
	}

	if err := r.RunInstaller(ctx, phpBinary, installerPath, tempDir); err != nil {
		cleanup()
		return nil, composerDownloadError(err)
	}
	if err := os.Remove(installerPath); err != nil {
		cleanup()
		return nil, composerDownloadError(fmt.Errorf("remove verified installer: %w", err))
	}

	composerPath := filepath.Join(tempDir, "composer.phar")
	if info, err := os.Stat(composerPath); err != nil || info.IsDir() {
		cleanup()
		if err != nil {
			return nil, composerDownloadError(fmt.Errorf("Composer installer did not create composer.phar: %w", err))
		}
		return nil, composerDownloadError(fmt.Errorf("Composer installer did not create composer.phar"))
	}
	_ = os.Chmod(composerPath, 0o600)

	return &Composer{Path: composerPath, PHPBinary: phpBinary, Temporary: true, cleanup: cleanup}, nil
}

func composerDownloadError(err error) error {
	return fmt.Errorf("Composer was not found and a temporary Composer download failed: %w; install Composer or ensure outbound HTTPS access", err)
}

func (r *ComposerResolver) downloadInstaller(ctx context.Context, installerPath string) (string, error) {
	installerURL, err := trustedHTTPSURL(r.InstallerURL)
	if err != nil {
		return "", err
	}
	signatureURL, err := trustedHTTPSURL(r.InstallerSignatureURL)
	if err != nil {
		return "", err
	}

	installer, err := download(ctx, r.HTTPClient, installerURL, 10<<20)
	if err != nil {
		return "", fmt.Errorf("download installer: %w", err)
	}
	defer installer.Close()

	file, err := os.OpenFile(installerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, installer); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}

	signature, err := download(ctx, r.HTTPClient, signatureURL, 1024)
	if err != nil {
		return "", fmt.Errorf("download installer signature: %w", err)
	}
	defer signature.Close()

	value, err := io.ReadAll(signature)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(value)), nil
}

func trustedHTTPSURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, fmt.Errorf("untrusted Composer download URL")
	}
	return parsed, nil
}

func download(ctx context.Context, client *http.Client, endpoint *url.URL, limit int64) (io.ReadCloser, error) {
	if client == nil {
		return nil, fmt.Errorf("HTTP client is not configured")
	}
	secureClient := *client
	secureClient.CheckRedirect = func(req *http.Request, _ []*http.Request) error {
		if req.URL.Scheme != "https" || req.URL.Host != endpoint.Host {
			return fmt.Errorf("redirected to untrusted Composer download URL")
		}
		return nil
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := secureClient.Do(request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		_ = response.Body.Close()
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	return &limitedReadCloser{Reader: io.LimitReader(response.Body, limit+1), closer: response.Body, limit: limit}, nil
}

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
	limit  int64
	read   int64
}

func (r *limitedReadCloser) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.read += int64(n)
	if r.read > r.limit {
		return n, fmt.Errorf("download exceeds size limit")
	}
	return n, err
}

func (r *limitedReadCloser) Close() error {
	return r.closer.Close()
}

func verifyInstaller(installerPath, signature string) error {
	expected, err := hex.DecodeString(signature)
	if err != nil || len(expected) != sha512.Size384 {
		return fmt.Errorf("malformed installer signature")
	}
	installer, err := os.ReadFile(installerPath)
	if err != nil {
		return err
	}
	actual := sha512.Sum384(installer)
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return fmt.Errorf("installer checksum mismatch")
	}
	return nil
}

func runComposerInstaller(ctx context.Context, phpBinary, installerPath, tempDir string) error {
	cmd := exec.CommandContext(ctx, phpBinary, installerPath, "--install-dir="+tempDir, "--filename=composer.phar", "--quiet")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("run Composer installer: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
