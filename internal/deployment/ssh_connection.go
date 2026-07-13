package deployment

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/sshcmd"
	"github.com/shopware/shopware-cli/logging"
)

// sshConnection runs remote commands through the system ssh client. Together
// with the ControlMaster options from sshcmd all commands of a deployment (and
// remote command execution) share one multiplexed connection per host.
type sshConnection struct {
	cfg *shop.EnvironmentSSH
}

func newSSHConnection(cfg *shop.EnvironmentSSH) (*sshConnection, error) {
	if cfg == nil || cfg.Host == "" {
		return nil, fmt.Errorf("the environment is missing the ssh.host setting")
	}

	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, fmt.Errorf("the ssh client was not found in PATH: %w", err)
	}

	return &sshConnection{cfg: cfg}, nil
}

func (c *sshConnection) Run(ctx context.Context, command string) (string, error) {
	logging.FromContext(ctx).Debugf("ssh %s: %s", c.cfg.Host, command)

	cmd := sshcmd.Build(ctx, c.cfg, command)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("remote command %q failed: %w\n%s", command, err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}

func (c *sshConnection) Stream(ctx context.Context, command string, stdin io.Reader) error {
	logging.FromContext(ctx).Debugf("ssh %s (stream): %s", c.cfg.Host, command)

	cmd := sshcmd.Build(ctx, c.cfg, command)
	cmd.Stdin = stdin

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remote command %q failed: %w\n%s", command, err, strings.TrimSpace(string(output)))
	}

	return nil
}

func (c *sshConnection) Close() error {
	// nothing to close: the multiplexed master connection is managed by the
	// ssh client itself and lingers (ControlPersist) so follow-up commands
	// like "project console" reuse it
	return nil
}
