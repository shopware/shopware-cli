package proxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/shop"
)

// ApplyRandomPorts remaps every conflicting host port to a random free one:
// it regenerates compose.yaml with the overrides and persists them to the
// local config override so future runs reuse them. The overrides come back in
// conflict order. The returned config carries them; cfg itself is left
// untouched so callers can adopt the result only once every write succeeded.
func ApplyRandomPorts(ctx context.Context, projectRoot, configPath string, cfg *shop.Config, proxyFallback bool, conflicts []docker.PortConflict) (*shop.Config, []docker.PortOverride, error) {
	overrides, err := docker.AllocateRandomPorts(ctx, conflicts)
	if err != nil {
		return nil, nil, err
	}

	updated := cfg.WithDockerPortOverrides(overrides)

	env, err := NewEnvironment(projectRoot, updated, proxyFallback)
	if err != nil {
		return nil, nil, err
	}

	// Both writes succeed or neither sticks: a compose file the persisted
	// config cannot reproduce would serve the new ports once and lose them on
	// the next regeneration.
	restoreCompose, err := snapshotFile(filepath.Join(projectRoot, "compose.yaml"))
	if err != nil {
		return nil, nil, err
	}
	if err := env.WriteCompose(); err != nil {
		return nil, nil, err
	}

	if err := shop.UpdateLocalDockerPorts(configPath, overrides); err != nil {
		if restoreErr := restoreCompose(); restoreErr != nil {
			return nil, nil, fmt.Errorf("%w (and restoring compose.yaml failed: %v)", err, restoreErr)
		}
		return nil, nil, err
	}

	return updated, overrides, nil
}

// snapshotFile captures the current content of path and returns a function
// that puts it back: the previous bytes when the file existed, otherwise the
// file is removed again.
func snapshotFile(path string) (func() error, error) {
	previous, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return func() error {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return func() error { return os.WriteFile(path, previous, 0o644) }, nil
}
