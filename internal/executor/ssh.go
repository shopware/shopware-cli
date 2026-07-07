package executor

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shell"
	"github.com/shopware/shopware-cli/internal/shop"
)

// SSHExecutor runs commands on the primary host of an ssh environment inside
// the currently deployed release (<deployment.path>/current). It wraps the
// system ssh client, so the user's SSH agent and configuration are honored and
// interactive commands work like a plain ssh session.
type SSHExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
}

// newSSHExecutor validates the environment and returns an SSHExecutor.
func newSSHExecutor(projectRoot string, envCfg *shop.EnvironmentConfig, shopCfg *shop.Config) (*SSHExecutor, error) {
	if envCfg.SSH == nil || envCfg.SSH.Host == "" {
		return nil, fmt.Errorf("the environment is missing the ssh.host setting")
	}

	if envCfg.Deployment == nil || envCfg.Deployment.Path == "" {
		return nil, fmt.Errorf("the environment is missing the deployment.path setting")
	}

	return &SSHExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: envCfg}, nil
}

// stdinIsTerminal is a variable so tests produce stable ssh arguments.
var stdinIsTerminal = func() bool {
	return isatty.IsTerminal(os.Stdin.Fd())
}

func (s *SSHExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, append([]string{"php", consoleCommandName(ctx)}, args...))
}

func (s *SSHExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, append([]string{"composer"}, args...))
}

func (s *SSHExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, append([]string{"php"}, args...))
}

func (s *SSHExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, append([]string{"npm"}, args...))
}

func (s *SSHExecutor) command(ctx context.Context, argv []string) *Process {
	sshArgs := s.baseArgs()
	sshArgs = append(sshArgs, s.destination(), s.remoteCommand(argv))

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	logCmd(ctx, cmd)

	return newProcess(cmd)
}

func (s *SSHExecutor) baseArgs() []string {
	cfg := s.envCfg.SSH

	var args []string

	if cfg.Port != 0 {
		args = append(args, "-p", strconv.Itoa(cfg.Port))
	}

	if cfg.IdentityFile != "" {
		args = append(args, "-i", cfg.IdentityFile)
	}

	if cfg.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+cfg.KnownHostsFile)
	}

	if cfg.InsecureIgnoreHostKey {
		args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
	}

	// allocate a TTY when we have one, so interactive console commands
	// (prompts, progress bars, colors) behave like a plain ssh session
	if stdinIsTerminal() {
		args = append(args, "-t")
	}

	return args
}

func (s *SSHExecutor) destination() string {
	cfg := s.envCfg.SSH

	if cfg.User != "" {
		return cfg.User + "@" + cfg.Host
	}

	return cfg.Host
}

// remoteBase is the root of the currently deployed release on the server.
func (s *SSHExecutor) remoteBase() string {
	return path.Join(strings.TrimRight(s.envCfg.Deployment.Path, "/"), "current")
}

func (s *SSHExecutor) remoteWorkdir() string {
	if s.relDir == "" {
		return s.remoteBase()
	}

	return path.Join(s.remoteBase(), filepath.ToSlash(s.relDir))
}

// remoteCommand builds the shell command executed on the server: change into
// the deployed release, apply the environment and run the quoted argv.
func (s *SSHExecutor) remoteCommand(argv []string) string {
	parts := make([]string, 0, len(s.env)+len(argv))

	for _, k := range slices.Sorted(maps.Keys(s.env)) {
		parts = append(parts, k+"="+shell.Quote(s.env[k]))
	}

	for _, arg := range argv {
		parts = append(parts, shell.Quote(arg))
	}

	return "cd " + shell.Quote(s.remoteWorkdir()) + " && " + strings.Join(parts, " ")
}

func (s *SSHExecutor) NormalizePath(hostPath string) string {
	if s.projectRoot == "" {
		return hostPath
	}

	rel, err := filepath.Rel(s.projectRoot, hostPath)
	if err != nil {
		return hostPath
	}

	return path.Join(s.remoteBase(), filepath.ToSlash(rel))
}

func (s *SSHExecutor) Type() string {
	return TypeSSH
}

func (s *SSHExecutor) WithEnv(env map[string]string) Executor {
	projectRootEnv := []string{"PROJECT_ROOT", "ADMIN_ROOT", "STOREFRONT_ROOT"}

	for _, k := range projectRootEnv {
		if _, ok := env[k]; ok {
			if strings.HasPrefix(env[k], s.projectRoot) {
				env[k] = s.NormalizePath(env[k])
			}
		}
	}

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

// EnvironmentStatus reports whether a release is deployed and linked on the server.
func (s *SSHExecutor) EnvironmentStatus(ctx context.Context) (bool, error) {
	sshArgs := s.baseArgs()
	sshArgs = append(sshArgs, s.destination(), "test -e "+shell.Quote(s.remoteBase()))

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	logCmd(ctx, cmd)

	if err := cmd.Run(); err != nil {
		return false, nil
	}

	return true, nil
}
