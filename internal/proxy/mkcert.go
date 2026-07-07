package proxy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// mkcert (https://github.com/FiloSottile/mkcert) cannot be imported as a
// library, but when the binary is installed we reuse it: certificates are
// issued by the mkcert root CA, which is usually already trusted on the
// machine (mkcert -install), so no additional trust prompt is needed.

// MkcertAvailable reports whether the mkcert binary can be used.
func MkcertAvailable() bool {
	if os.Getenv("SHOPWARE_CLI_PROXY_DISABLE_MKCERT") != "" {
		return false
	}

	_, err := exec.LookPath("mkcert")

	return err == nil
}

// MkcertCAPath returns the path of the mkcert root CA certificate.
func MkcertCAPath(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "mkcert", "-CAROOT")

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkcert -CAROOT: %w", err)
	}

	caroot := strings.TrimSpace(string(output))
	if caroot == "" {
		return "", fmt.Errorf("mkcert -CAROOT returned an empty path")
	}

	return filepath.Join(caroot, "rootCA.pem"), nil
}

// generateWithMkcert issues the server certificate for the given hosts using
// mkcert. mkcert creates its root CA automatically on first use.
func generateWithMkcert(ctx context.Context, dir string, hosts []string) error {
	if err := os.MkdirAll(filepath.Dir(ServerCertPath(dir)), 0o700); err != nil {
		return err
	}

	args := []string{"-cert-file", ServerCertPath(dir), "-key-file", ServerKeyPath(dir)}
	args = append(args, hosts...)

	cmd := exec.CommandContext(ctx, "mkcert", args...)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mkcert %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}

	return nil
}
