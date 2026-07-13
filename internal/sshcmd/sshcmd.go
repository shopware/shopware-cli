// Package sshcmd builds commands for the system ssh client from an
// environment's SSH settings. It is the single SSH transport used by both
// deployments and remote command execution, so all of them share one
// multiplexed connection (ControlMaster) per host.
package sshcmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
)

// ControlSocketDir returns the directory for ControlMaster sockets. It is a
// variable so tests produce stable arguments.
var ControlSocketDir = func() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// ~/.ssh keeps the socket path short (unix sockets have a ~100 byte path
	// limit) and follows the OpenSSH convention for multiplexing sockets
	dir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}

	return dir
}

// sshpassPath locates sshpass for password based authentication. It is a
// variable so tests can stub it.
var sshpassPath = func() string {
	path, err := exec.LookPath("sshpass")
	if err != nil {
		return ""
	}

	return path
}

// controlMasterEnabled reports whether SSH connection multiplexing should be
// used. Windows' OpenSSH build does not support ControlMaster.
func controlMasterEnabled(cfg *shop.EnvironmentSSH) bool {
	if runtime.GOOS == "windows" {
		return false
	}

	if cfg.ControlMaster != nil {
		return *cfg.ControlMaster
	}

	return true
}

// BaseArgs returns the ssh client arguments derived from the SSH settings.
func BaseArgs(cfg *shop.EnvironmentSSH) []string {
	var args []string

	if cfg.Port != 0 {
		args = append(args, "-p", strconv.Itoa(cfg.Port))
	}

	if cfg.IdentityFile != "" {
		args = append(args, "-i", expandHome(cfg.IdentityFile))
	}

	if cfg.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+expandHome(cfg.KnownHostsFile))
	}

	if cfg.InsecureIgnoreHostKey {
		args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
	}

	// reuse one connection for consecutive commands instead of paying the
	// TCP + key exchange + authentication handshake on every invocation.
	// %C hashes local host, remote user, host and port into a short name.
	if controlMasterEnabled(cfg) {
		if socketDir := ControlSocketDir(); socketDir != "" {
			args = append(args,
				"-o", "ControlMaster=auto",
				"-o", "ControlPath="+filepath.Join(socketDir, "shopware-cli-%C"),
				"-o", "ControlPersist=60",
			)
		}
	}

	return args
}

// Destination returns the [user@]host argument for the ssh client.
func Destination(cfg *shop.EnvironmentSSH) string {
	if cfg.User != "" {
		return cfg.User + "@" + cfg.Host
	}

	return cfg.Host
}

// Build returns an exec.Cmd running the given command on the host over ssh.
// When a password is configured and sshpass is available, it is used so the
// password does not have to be typed interactively.
func Build(ctx context.Context, cfg *shop.EnvironmentSSH, remoteCommand string, extraArgs ...string) *exec.Cmd {
	args := BaseArgs(cfg)
	args = append(args, extraArgs...)
	args = append(args, Destination(cfg), remoteCommand)

	if cfg.Password != "" {
		if sshpass := sshpassPath(); sshpass != "" {
			cmd := exec.CommandContext(ctx, sshpass, append([]string{"-e", "ssh"}, args...)...)
			cmd.Env = append(os.Environ(), "SSHPASS="+cfg.Password)

			return cmd
		}
	}

	return exec.CommandContext(ctx, "ssh", args...)
}

// Output runs a command on the host and returns its stdout.
func Output(ctx context.Context, cfg *shop.EnvironmentSSH, remoteCommand string) (string, error) {
	cmd := Build(ctx, cfg, remoteCommand)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("remote command %q failed: %w\n%s", remoteCommand, err, bytes.TrimSpace(stderr.Bytes()))
	}

	return string(output), nil
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}

		return filepath.Join(home, strings.TrimPrefix(path[1:], string(os.PathSeparator)))
	}

	return path
}
