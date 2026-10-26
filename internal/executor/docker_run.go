package executor

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

// ErrDockerCleanup indicates that a disposable container might still be running.
var ErrDockerCleanup = errors.New("cannot remove Docker container")

// DockerRunCommand creates a disposable container with a unique name and waits
// for its removal on cancellation before callers continue.
func DockerRunCommand(ctx context.Context, args ...string) (*exec.Cmd, func(error) error) {
	name := "shopware-cli-run-" + rand.Text()
	args = append([]string{"run", "--rm", "--name", name}, args...)
	cmd := exec.CommandContext(ctx, "docker", args...)
	var once sync.Once
	var cleanupErr error
	cmd.Cancel = func() error {
		once.Do(func() {
			logging.FromContext(ctx).Info("Stopping Docker container")
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
		// cancelled context. Stop it on every unsuccessful run before continuing.
		if err != nil && cmd.Process != nil {
			_ = cmd.Cancel()
		}
		return errors.Join(err, cleanupErr)
	}
}
