package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
)

// SSHExecutor runs commands on a remote target over SSH and provides a
// Deployer that performs release-based deployments via rsync + ssh.
//
// Local commands (composer, npm, ...) are not executed locally; the executor
// shells out to ssh user@host so that operations like `project console` can
// be aimed at the remote shop's current release. When multiple hosts are
// configured, executor commands target the first (primary) host.
type SSHExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
}

func (s *SSHExecutor) sshConfig() *shop.EnvironmentSSHConfig {
	if s.envCfg == nil {
		return nil
	}
	return s.envCfg.SSH
}

// resolvedHosts returns the effective SSH targets for this executor.
func (s *SSHExecutor) resolvedHosts() []shop.EnvironmentSSHHostConfig {
	return s.sshConfig().ResolvedHosts()
}

// primaryHost returns the first resolved host, used for executor commands
// like ConsoleCommand that target a single shop.
func (s *SSHExecutor) primaryHost() (shop.EnvironmentSSHHostConfig, bool) {
	hosts := s.resolvedHosts()
	if len(hosts) == 0 {
		return shop.EnvironmentSSHHostConfig{}, false
	}
	return hosts[0], true
}

// sshTargetFor returns the user@host form for an ssh invocation.
func sshTargetFor(h shop.EnvironmentSSHHostConfig) string {
	if h.User != "" {
		return h.User + "@" + h.Host
	}
	return h.Host
}

// sshArgsFor returns the leading arguments for an `ssh` invocation to a
// specific host (port, identity file, target). Trailing remote command
// words should be appended by the caller.
func sshArgsFor(h shop.EnvironmentSSHHostConfig) []string {
	args := []string{"-o", "BatchMode=yes"}
	if h.Port != 0 && h.Port != 22 {
		args = append(args, "-p", strconv.Itoa(h.Port))
	}
	if h.IdentityFile != "" {
		args = append(args, "-i", expandHome(h.IdentityFile))
	}
	args = append(args, sshTargetFor(h))
	return args
}

// remoteCommand wraps a shell command to be executed inside the current release
// directory of the primary host, applying the executor's env and relDir.
func (s *SSHExecutor) remoteCommand(ctx context.Context, command string) *exec.Cmd {
	host, ok := s.primaryHost()
	workdir := ""
	if ok {
		workdir = path.Join(host.DeployPath, "current")
		if s.relDir != "" {
			workdir = path.Join(workdir, s.relDir)
		}
	}

	var b strings.Builder
	if workdir != "" {
		fmt.Fprintf(&b, "cd %s && ", shellQuote(workdir))
	}
	for k, v := range s.env {
		fmt.Fprintf(&b, "%s=%s ", k, shellQuote(v))
	}
	b.WriteString(command)

	args := append(sshArgsFor(host), b.String())
	cmd := exec.CommandContext(ctx, "ssh", args...)
	logCmd(ctx, cmd)
	return cmd
}

func (s *SSHExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	parts := append([]string{"php", consoleCommandName(ctx)}, args...)
	return newProcess(s.remoteCommand(ctx, joinShell(parts)))
}

func (s *SSHExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	parts := append([]string{"composer"}, args...)
	return newProcess(s.remoteCommand(ctx, joinShell(parts)))
}

func (s *SSHExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	parts := append([]string{"php"}, args...)
	return newProcess(s.remoteCommand(ctx, joinShell(parts)))
}

func (s *SSHExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	parts := append([]string{"npm"}, args...)
	return newProcess(s.remoteCommand(ctx, joinShell(parts)))
}

func (s *SSHExecutor) NormalizePath(hostPath string) string {
	return hostPath
}

func (s *SSHExecutor) Type() string {
	return TypeSSH
}

func (s *SSHExecutor) WithEnv(env map[string]string) Executor {
	return &SSHExecutor{env: mergeEnv(s.env, env), projectRoot: s.projectRoot, relDir: s.relDir, shopCfg: s.shopCfg, envCfg: s.envCfg}
}

func (s *SSHExecutor) WithRelDir(relDir string) Executor {
	return &SSHExecutor{env: s.env, projectRoot: s.projectRoot, relDir: relDir, shopCfg: s.shopCfg, envCfg: s.envCfg}
}

func (s *SSHExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, s.shopCfg, s.envCfg)
}

func (s *SSHExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (s *SSHExecutor) StopEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (s *SSHExecutor) Deployer() Deployer {
	hosts := s.resolvedHosts()
	if len(hosts) == 0 {
		return nil
	}
	for _, h := range hosts {
		if h.Host == "" || h.DeployPath == "" {
			return nil
		}
	}
	return &SSHDeployer{exec: s, projectRoot: s.projectRoot, shopCfg: s.shopCfg, envCfg: s.envCfg, sshCfg: s.sshConfig()}
}

func expandHome(p string) string {
	if !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return path.Join(home, p[2:])
}

// shellQuote wraps s in single quotes and escapes embedded single quotes
// so it can be safely interpolated into a `sh -c` style remote command.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func joinShell(parts []string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = shellQuote(p)
	}
	return strings.Join(quoted, " ")
}
