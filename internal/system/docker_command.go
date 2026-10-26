package system

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

// ErrDockerCleanup indicates that a container might still be writing to its mounts.
var ErrDockerCleanup = errors.New("cannot stop installation container")

// DockerRunCommand gives a disposable container a unique name and waits for its
// removal on cancellation, before callers can remove any bind-mounted files.
func DockerRunCommand(ctx context.Context, args ...string) (*exec.Cmd, func(error) error) {
	name := "shopware-cli-install-" + rand.Text()
	args = append([]string{"run", "--name", name}, args...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	var once sync.Once
	var cleanupErr error
	cmd.Cancel = func() error {
		once.Do(func() {
			logging.FromContext(ctx).Info("Stopping installation container")
			// Stop the client first so a pending pull cannot subsequently start a container.
			_ = cmd.Process.Kill()
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			output, err := exec.CommandContext(cleanupCtx, "docker", "rm", "--force", name).CombinedOutput()
			if err != nil && !strings.Contains(string(output), "No such container") {
				cleanupErr = fmt.Errorf("%w: %s: %w", ErrDockerCleanup, strings.TrimSpace(string(output)), err)
			}
		})
		return cleanupErr
	}
	return cmd, func(err error) error {
		// A disconnected client can also leave its container running without a
		// cancelled context. Stop it on every unsuccessful run before rollback.
		if err != nil && cmd.Process != nil {
			_ = cmd.Cancel()
		}
		return errors.Join(err, cleanupErr)
	}
}
