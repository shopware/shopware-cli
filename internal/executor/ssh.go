package executor

import (
	"context"
	"fmt"
	"maps"
	"net"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
	"github.com/mattn/go-isatty"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shell"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/sshcmd"
	"github.com/shopware/shopware-cli/logging"
)

// SSHExecutor runs commands on the primary host of an ssh environment inside
// the currently deployed release (<deployment.path>/current). It wraps the
// system ssh client, so the user's SSH agent and configuration are honored,
// interactive commands work like a plain ssh session and the multiplexed
// connection (ControlMaster) is shared with deployments.
type SSHExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig

	// readEnvFiles is injected for tests
	readEnvFiles func(ctx context.Context, names ...string) (map[string]string, error)
}

// newSSHExecutor validates the environment and returns an SSHExecutor.
func newSSHExecutor(projectRoot string, envCfg *shop.EnvironmentConfig, shopCfg *shop.Config) (*SSHExecutor, error) {
	if envCfg.SSH == nil || envCfg.SSH.Host == "" {
		return nil, fmt.Errorf("the environment is missing the ssh.host setting")
	}

	if envCfg.SSH.Deployment == nil || envCfg.SSH.Deployment.Path == "" {
		return nil, fmt.Errorf("the environment is missing the ssh.deployment.path setting")
	}

	executor := &SSHExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: envCfg}
	executor.readEnvFiles = executor.readRemoteEnvFiles

	return executor, nil
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
	var extraArgs []string

	// allocate a TTY when we have one, so interactive console commands
	// (prompts, progress bars, colors) behave like a plain ssh session
	if stdinIsTerminal() {
		extraArgs = append(extraArgs, "-t")
	}

	cmd := sshcmd.Build(ctx, s.envCfg.SSH, s.remoteCommand(argv), extraArgs...)
	logCmd(ctx, cmd)

	return newProcess(cmd)
}

// remoteBase is the root of the currently deployed release on the server.
func (s *SSHExecutor) remoteBase() string {
	return path.Join(strings.TrimRight(s.envCfg.SSH.Deployment.Path, "/"), "current")
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

	return &SSHExecutor{env: mergeEnv(s.env, env), projectRoot: s.projectRoot, relDir: s.relDir, shopCfg: s.shopCfg, envCfg: s.envCfg, readEnvFiles: s.readEnvFiles}
}

func (s *SSHExecutor) WithRelDir(relDir string) Executor {
	return &SSHExecutor{env: s.env, projectRoot: s.projectRoot, relDir: relDir, shopCfg: s.shopCfg, envCfg: s.envCfg, readEnvFiles: s.readEnvFiles}
}

func (s *SSHExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, s.shopCfg, s.envCfg)
}

// sshMySQLNetwork is the network name registered with the MySQL driver for
// connections that are dialed through the ssh client.
const sshMySQLNetwork = "ssh+tcp"

// DatabaseConnection reads the deployed release's Symfony env files and
// returns a connection config whose transport is dialed through ssh, so the
// database is reachable from the local machine even when it only listens on
// the server.
func (s *SSHExecutor) DatabaseConnection(ctx context.Context) (*mysql.Config, error) {
	cfg := defaultMySQLConfig()

	env, err := s.remoteEnv(ctx)
	if err != nil {
		return nil, err
	}

	if databaseURL := env["DATABASE_URL"]; databaseURL != "" {
		logging.FromContext(ctx).Infof("Using DATABASE_URL of the deployed release. options can override specific parts (--username=foo)")

		if err := applyDatabaseURL(cfg, databaseURL); err != nil {
			return nil, err
		}
	} else {
		logging.FromContext(ctx).Warnf("No DATABASE_URL found in the deployed release's env files, using defaults. Use the connection options (--host, --username, ...) to override")
	}

	// the address stays the database address as seen from the server; the
	// registered dialer opens the stream through the ssh connection
	sshCfg := s.envCfg.SSH
	mysql.RegisterDialContext(sshMySQLNetwork, func(_ context.Context, addr string) (net.Conn, error) {
		return sshcmd.Dial(sshCfg, addr)
	})

	cfg.Net = sshMySQLNetwork

	return cfg, nil
}

// remoteEnv reads the Symfony env files of the deployed release with the same
// precedence as a local project (.env.dist < .env < .env.local, then the
// APP_ENV specific variants).
func (s *SSHExecutor) remoteEnv(ctx context.Context) (map[string]string, error) {
	base, err := s.readEnvFiles(ctx, ".env.dist", ".env", ".env.local")
	if err != nil {
		return nil, err
	}

	appEnv := base["APP_ENV"]
	if appEnv == "" {
		return base, nil
	}

	specific, err := s.readEnvFiles(ctx, ".env."+appEnv, ".env."+appEnv+".local")
	if err != nil {
		return nil, err
	}

	maps.Copy(base, specific)

	return base, nil
}

// readRemoteEnvFiles concatenates the given env files of the deployed release
// in order and parses them, so later files override earlier ones.
func (s *SSHExecutor) readRemoteEnvFiles(ctx context.Context, names ...string) (map[string]string, error) {
	reads := make([]string, 0, len(names))
	for _, name := range names {
		// the echo separates files that do not end with a newline
		reads = append(reads, fmt.Sprintf("cat %s 2>/dev/null; echo", shell.Quote(path.Join(s.remoteBase(), name))))
	}

	output, err := sshcmd.Output(ctx, s.envCfg.SSH, strings.Join(reads, "; "))
	if err != nil {
		return nil, fmt.Errorf("cannot read env files of the deployed release: %w", err)
	}

	env, err := godotenv.Unmarshal(output)
	if err != nil {
		return nil, fmt.Errorf("cannot parse env files of the deployed release: %w", err)
	}

	return env, nil
}

func (s *SSHExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (s *SSHExecutor) StopEnvironment(_ context.Context) error {
	return ErrNotSupported
}

// EnvironmentStatus reports whether a release is deployed and linked on the server.
func (s *SSHExecutor) EnvironmentStatus(ctx context.Context) (bool, error) {
	cmd := sshcmd.Build(ctx, s.envCfg.SSH, "test -e "+shell.Quote(s.remoteBase()))
	logCmd(ctx, cmd)

	// a non-zero exit of test -e means no release is deployed, not a failure
	if err := cmd.Run(); err != nil {
		return false, nil //nolint:nilerr
	}

	return true, nil
}
