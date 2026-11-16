package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const caBundleDirName = "ca-bundles"

// systemCABundlePath is where Debian-based images keep their trust store; the
// combined bundle is mounted over it. Must match docker.containerCABundlePath.
const systemCABundlePath = "/etc/ssl/certs/ca-certificates.crt"

// ContainerCABundlePath is the deterministic host path of the combined CA
// bundle for image (its system CAs plus the proxy root CA). It is keyed by the
// image tag so different PHP/Node images keep separate bundles. The path is
// pure: EnsureContainerCABundle writes the file, while the compose builders only
// reference the path — the file is guaranteed to exist by PrepareInfra before
// the containers start.
func ContainerCABundlePath(image string) (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256([]byte(image))

	return filepath.Join(dir, caBundleDirName, hex.EncodeToString(sum[:8])+".crt"), nil
}

// EnsureContainerCABundle writes (or refreshes) the combined CA bundle for
// image: the image's own system trust store with the proxy root CA appended.
// Mounting it over /etc/ssl/certs/ca-certificates.crt keeps the container
// trusting the public internet AND the proxy's HTTPS certificates. Just mounting
// the bare CA under /usr/local/share/ca-certificates does nothing — the image
// runs as www-data and never runs update-ca-certificates. It rebuilds when the
// bundle is missing or older than the CA, so a regenerated CA propagates (an
// image gaining new public CAs is not detected; that is a rare, benign staleness
// callers refresh by deleting the state dir).
func EnsureContainerCABundle(ctx context.Context, image, caPath string) (string, error) {
	bundlePath, err := ContainerCABundlePath(image)
	if err != nil {
		return "", err
	}

	caPEM, err := os.ReadFile(caPath)
	if err != nil {
		return "", fmt.Errorf("reading proxy CA: %w", err)
	}

	if bundleFresh(bundlePath, caPath) {
		return bundlePath, nil
	}

	systemPEM, err := imageSystemCABundle(ctx, image)
	if err != nil {
		return "", err
	}

	combined := combineCABundle(systemPEM, caPEM)

	if err := os.MkdirAll(filepath.Dir(bundlePath), 0o700); err != nil {
		return "", err
	}

	// 0o644: the bundle is public data and the container's www-data user must be
	// able to read it through the read-only bind mount.
	if err := writeFileAtomic(bundlePath, combined, 0o644); err != nil {
		return "", err
	}

	return bundlePath, nil
}

// combineCABundle appends the proxy CA to the image's system bundle, separated
// by a newline so the two PEM blocks never run together if the system bundle
// lacks a trailing newline.
func combineCABundle(system, ca []byte) []byte {
	return append(append(append([]byte{}, system...), '\n'), ca...)
}

// bundleFresh reports whether the bundle exists and is at least as new as the
// CA, so it does not need rebuilding.
func bundleFresh(bundlePath, caPath string) bool {
	bi, err := os.Stat(bundlePath)
	if err != nil {
		return false
	}

	ci, err := os.Stat(caPath)
	if err != nil {
		return false
	}

	return !bi.ModTime().Before(ci.ModTime())
}

// imageSystemCABundle reads the system CA bundle straight out of image, so the
// combined bundle matches exactly the public CAs that image ships. Its own
// entrypoint is bypassed (--entrypoint cat) and only stdout is used, so the
// output is the PEM file and nothing else.
func imageSystemCABundle(ctx context.Context, image string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--entrypoint", "cat", image, systemCABundlePath)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("reading system CA bundle from %s: %w\n%s", image, err, stderr.String())
	}

	if stdout.Len() == 0 {
		return nil, fmt.Errorf("system CA bundle from %s is empty", image)
	}

	return stdout.Bytes(), nil
}

// writeFileAtomic writes data to path via a temp file in the same directory
// renamed into place, so a reader (or a bind mount) never sees a partial file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once renamed

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}
