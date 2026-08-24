package executor

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mattn/go-isatty"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// hostStdinStdoutAreTerminals reports whether the current process is attached
// to a terminal on both stdin and stdout. Overridden in tests.
var hostStdinStdoutAreTerminals = func() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}

type DockerExecutor struct {
	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
	// composeProjectName pins the Compose project (-p) for every invocation.
	// It is resolved from the project .env once at construction: Compose
	// re-reads .env per command, so a mid-run rewrite of that file (e.g.
	// `composer recipes:install --force --reset` during an upgrade) would
	// otherwise silently disconnect running containers.
	composeProjectName string
}

// composeArgs starts a docker argument list for a compose subcommand, pinning
// the project name when one is known.
func (d *DockerExecutor) composeArgs(sub ...string) []string {
	args := []string{"compose"}
	if d.composeProjectName != "" {
		args = append(args, "-p", d.composeProjectName)
	}
	return append(args, sub...)
}

func (d *DockerExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "php", consoleCommandName(ctx))
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"php", consoleCommandName(ctx)}, args...))
}

func (d *DockerExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "composer")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"composer"}, args...))
}

func (d *DockerExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "php")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"php"}, args...))
}

func (d *DockerExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	dockerArgs := d.baseArgs()
	dockerArgs = append(dockerArgs, "env-bridge", "npm")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	applyDir(d.projectRoot, cmd)
	logCmd(ctx, cmd)
	return d.newProcess(cmd, append([]string{"npm"}, args...))
}

func (d *DockerExecutor) NormalizePath(hostPath string) string {
	if d.projectRoot == "" {
		return hostPath
	}

	rel, err := filepath.Rel(d.projectRoot, hostPath)
	if err != nil {
		return hostPath
	}

	return filepath.Join("/var/www/html", rel)
}

func (d *DockerExecutor) Type() string {
	return TypeDocker
}

func (d *DockerExecutor) WithEnv(env map[string]string) Executor {
	projectRootEnv := []string{"PROJECT_ROOT", "ADMIN_ROOT", "STOREFRONT_ROOT"}

	for _, k := range projectRootEnv {
		if _, ok := env[k]; ok {
			if strings.HasPrefix(env[k], d.projectRoot) {
				env[k] = d.NormalizePath(env[k])
			}
		}
	}

	return &DockerExecutor{env: mergeEnv(d.env, env), projectRoot: d.projectRoot, relDir: d.relDir, shopCfg: d.shopCfg, envCfg: d.envCfg, composeProjectName: d.composeProjectName}
}

func (d *DockerExecutor) WithRelDir(relDir string) Executor {
	return &DockerExecutor{env: d.env, projectRoot: d.projectRoot, relDir: relDir, shopCfg: d.shopCfg, envCfg: d.envCfg, composeProjectName: d.composeProjectName}
}

func (d *DockerExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, d.shopCfg, d.envCfg)
}

func (d *DockerExecutor) ShopConfig() *shop.Config {
	return d.shopCfg
}

// DatabaseConnection resolves the database credentials as seen inside the
// compose network and translates the service host to the port published on
// the host machine.
func (d *DockerExecutor) DatabaseConnection(ctx context.Context) (*DatabaseConnection, error) {
	conn := defaultDatabaseConnection()
	conn.Host = "database"

	databaseURL := d.env["DATABASE_URL"]

	if databaseURL == "" {
		// Always disable TTY: this captures stdout and is never interactive.
		cmd := exec.CommandContext(ctx, "docker", "compose", "exec", "-T", "web", "printenv", "DATABASE_URL")
		cmd.Dir = d.projectRoot
		logCmd(ctx, cmd)

		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("could not read DATABASE_URL from the web container, is the environment running?: %w\n%s", err, stderr.String())
		}

		databaseURL = strings.TrimSpace(stdout.String())
	}

	if databaseURL != "" {
		if err := applyDatabaseURL(conn, databaseURL); err != nil {
			return nil, err
		}
	}

	// The host part of DATABASE_URL is only resolvable inside the compose
	// network when it names a compose service. Swap it for the address the
	// port is published on.
	if err := d.resolvePublishedPort(ctx, conn); err != nil {
		return nil, err
	}

	return conn, nil
}

// resolvePublishedPort rewrites conn's address to the host-published mapping
// of the compose service it points at. When the host is not a compose service
// (external database), the address is kept untouched.
func (d *DockerExecutor) resolvePublishedPort(ctx context.Context, conn *DatabaseConnection) error {
	cmd := exec.CommandContext(ctx, "docker", "compose", "port", conn.Host, conn.Port)
	cmd.Dir = d.projectRoot
	logCmd(ctx, cmd)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "no such service") {
			return nil
		}

		return fmt.Errorf("could not resolve published port of service %q: %w\n%s", conn.Host, err, stderr.String())
	}

	published := strings.TrimSpace(stdout.String())
	if line, _, found := strings.Cut(published, "\n"); found {
		published = strings.TrimSpace(line)
	}

	host, port, err := net.SplitHostPort(published)
	if err != nil || port == "0" {
		return fmt.Errorf("service %q does not publish port %s to the host, regenerate the compose file by restarting the environment (shopware-cli project dev)", conn.Host, conn.Port)
	}

	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "127.0.0.1"
	}

	conn.Host = host
	conn.Port = port

	return nil
}

func (d *DockerExecutor) containerWorkdir() string {
	if d.relDir == "" {
		return "/var/www/html"
	}

	return filepath.Join("/var/www/html", d.relDir)
}

func (d *DockerExecutor) newProcess(cmd *exec.Cmd, innerArgs []string) *Process {
	projectRoot := d.projectRoot
	pattern := strings.Join(innerArgs, " ")

	return &Process{
		Cmd: cmd,
		stop: func(ctx context.Context) error {
			// Signal the whole in-container process tree rooted at the
			// matched command, not just the top process. npm (and similar
			// wrappers) spawn the actual long-running server — e.g. the Vite
			// dev server — as a child that does not receive npm's signal, so
			// signalling only the parent orphans it and it keeps holding its
			// port. pkill matches by pattern and would miss those children.
			// Always disable TTY: this is a fire-and-forget cleanup command.
			killArgs := append(d.composeArgs("exec", "-T", "web"), "sh", "-c", killTreeScript(pattern))
			killCmd := exec.CommandContext(ctx, "docker", killArgs...)
			killCmd.Dir = projectRoot
			_ = killCmd.Run()

			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGINT)
			}

			return nil
		},
	}
}

// killTreeScript builds a POSIX-sh snippet that SIGINTs every process whose
// command line matches pattern together with all of its descendants.
func killTreeScript(pattern string) string {
	// pgrep -f matches against the whole command line, so this very `sh -c`
	// script (which embeds the pattern) matches itself; skip our own PID ($$),
	// or kill_tree would terminate the cleanup shell before the real targets.
	return "kill_tree() { for c in $(pgrep -P \"$1\" 2>/dev/null); do kill_tree \"$c\"; done; kill -INT \"$1\" 2>/dev/null; }; " +
		"for p in $(pgrep -f " + shellSingleQuote(pattern) + " 2>/dev/null); do [ \"$p\" = \"$$\" ] && continue; kill_tree \"$p\"; done"
}

// shellSingleQuote wraps s in single quotes safely for embedding in an sh -c
// script.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (d *DockerExecutor) StartEnvironment(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "docker", d.composeArgs("up", "-d")...)
	cmd.Dir = d.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

func (d *DockerExecutor) StopEnvironment(ctx context.Context, opts StopOptions) error {
	downArgs := d.composeArgs("down")
	if opts.RemoveVolumes {
		downArgs = append(downArgs, "--volumes")
	}

	cmd := exec.CommandContext(ctx, "docker", downArgs...)
	cmd.Dir = d.projectRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\n%s", err, output)
	}

	return nil
}

func (d *DockerExecutor) EnvironmentStatus(ctx context.Context) (bool, error) {
	cmd := exec.CommandContext(ctx, "docker", d.composeArgs("ps", "--status=running", "-q")...)
	cmd.Dir = d.projectRoot

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("checking environment status: %w", err)
	}

	return len(strings.TrimSpace(string(output))) > 0, nil
}

func (d *DockerExecutor) baseArgs() []string {
	args := d.composeArgs("exec")

	// Allocate a TTY for interactive terminals so Symfony console keeps ANSI
	// colors and prompts. Keep -T when stdin/stdout is not a terminal (CI,
	// pipes) or when a TUI like project dev owns the host terminal — compose
	// exec would otherwise steal the TTY from the TUI.
	if system.IsTUIActive() || !hostStdinStdoutAreTerminals() {
		args = append(args, "-T")
	}

	// When the web service runs as the mapped host user (see the compose
	// user: directive derived from system.ProjectUserSpec), that UID has no
	// passwd entry inside the image, so HOME is unset and tools like npm and
	// composer fall back to / and fail with EACCES. Point HOME at a writable
	// path, mirroring system.DockerRunUserArgs for the raw composer run.
	if system.ProjectUserSpec(d.projectRoot) != "" {
		args = append(args, "-e", "HOME=/tmp")
	}

	for k, v := range d.env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, "--workdir", d.containerWorkdir())

	args = append(args, "web")

	return args
}
