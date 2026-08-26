package executor

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
)

// SSHExecutor runs commands against a Shopware project on a remote host over
// SSH. Every invocation is a one-shot ssh call; repeated calls are cheap
// because they share a multiplexed master connection (ControlMaster /
// ControlPersist), so the TCP and authentication handshake happens only once.
type SSHExecutor struct {
	host         string
	user         string
	port         int
	directory    string
	identityFile string

	env         map[string]string
	projectRoot string
	relDir      string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
}

func (s *SSHExecutor) target() string {
	if s.user == "" {
		return s.host
	}

	return s.user + "@" + s.host
}

// controlPath returns the ssh multiplexing socket path. It is stable per
// target (and local user), so all executors pointing at the same host share
// one master connection.
func (s *SSHExecutor) controlPath() string {
	identity := strings.Join([]string{strconv.Itoa(os.Getuid()), s.user, s.host, strconv.Itoa(s.port), s.identityFile}, "\x00")
	sum := sha256.Sum256([]byte(identity))

	return filepath.Join(os.TempDir(), fmt.Sprintf("shopware-cli-ssh-%x.sock", sum[:8]))
}

// sshArgs returns the base ssh argument list: connection multiplexing for
// fast repeated calls, plus port and identity options.
func (s *SSHExecutor) sshArgs() []string {
	args := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + s.controlPath(),
		"-o", "ControlPersist=10m",
	}

	if s.port != 0 && s.port != 22 {
		args = append(args, "-p", strconv.Itoa(s.port))
	}

	if s.identityFile != "" {
		args = append(args, "-i", s.identityFile)
	}

	return args
}

// remoteDir returns the working directory on the remote host. Remote paths
// are always POSIX, independent of the local platform.
func (s *SSHExecutor) remoteDir() string {
	if s.relDir == "" {
		return s.directory
	}

	return path.Join(s.directory, s.relDir)
}

// remoteShell builds the shell snippet executed on the remote host: cd into
// the project directory, apply the executor env, then run the command with
// every argument shell-quoted.
func (s *SSHExecutor) remoteShell(command ...string) string {
	var b strings.Builder

	b.WriteString("cd ")
	b.WriteString(shellQuoteArg(s.remoteDir()))
	b.WriteString(" && ")

	keys := make([]string, 0, len(s.env))
	for k := range s.env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(shellQuoteArg(s.env[k]))
		b.WriteString(" ")
	}

	for i, c := range command {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(shellQuoteArg(c))
	}

	return b.String()
}

// command builds an ssh invocation running name with args on the remote host.
func (s *SSHExecutor) command(ctx context.Context, name string, args ...string) *Process {
	sshArgs := s.sshArgs()

	// Request a TTY only when the caller opted in via WithTTY (interactive
	// project console); TUI, CI, and piped usage stay non-interactive.
	if wantsTTY(ctx) {
		sshArgs = append(sshArgs, "-t")
	} else {
		sshArgs = append(sshArgs, "-T")
	}

	sshArgs = append(sshArgs, s.target(), s.remoteShell(append([]string{name}, args...)...))

	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	logCmd(ctx, cmd)

	return newProcess(cmd)
}

func (s *SSHExecutor) ConsoleCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, "php", append([]string{consoleCommandName(ctx)}, args...)...)
}

func (s *SSHExecutor) ComposerCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, "composer", args...)
}

func (s *SSHExecutor) PHPCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, "php", args...)
}

func (s *SSHExecutor) NPMCommand(ctx context.Context, args ...string) *Process {
	return s.command(ctx, "npm", args...)
}

// NormalizePath translates a path below the local project root into the
// project directory on the remote host, mirroring the Docker executor. Paths
// outside the project root (or all paths when no project root is known) are
// returned unchanged.
func (s *SSHExecutor) NormalizePath(hostPath string) string {
	if s.projectRoot == "" {
		return hostPath
	}

	rel, err := filepath.Rel(s.projectRoot, hostPath)
	if err != nil {
		return hostPath
	}

	return filepath.Join(s.directory, rel)
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

	return &SSHExecutor{host: s.host, user: s.user, port: s.port, directory: s.directory, identityFile: s.identityFile, env: mergeEnv(s.env, env), projectRoot: s.projectRoot, relDir: s.relDir, shopCfg: s.shopCfg, envCfg: s.envCfg}
}

func (s *SSHExecutor) WithRelDir(relDir string) Executor {
	return &SSHExecutor{host: s.host, user: s.user, port: s.port, directory: s.directory, identityFile: s.identityFile, env: s.env, projectRoot: s.projectRoot, relDir: relDir, shopCfg: s.shopCfg, envCfg: s.envCfg}
}

func (s *SSHExecutor) AdminAPIClient(ctx context.Context) (*adminSdk.Client, error) {
	return adminAPIClient(ctx, s.shopCfg, s.envCfg)
}

func (s *SSHExecutor) ShopConfig() *shop.Config {
	return s.shopCfg
}

func (s *SSHExecutor) StartEnvironment(_ context.Context) error {
	return ErrNotSupported
}

func (s *SSHExecutor) StopEnvironment(_ context.Context, _ StopOptions) error {
	return ErrNotSupported
}

func (s *SSHExecutor) EnvironmentStatus(_ context.Context) (bool, error) {
	return false, ErrNotSupported
}

// DatabaseConnection resolves the database credentials from the remote
// project and routes the connection through the SSH host: the resolved
// host:port lives in the remote network, so the mysql driver is pointed at a
// registered dialer network that forwards each connection with `ssh -W`.
// Every MySQL connection spawns its own channel on the multiplexed SSH
// connection, so no persistent tunnel process has to be managed.
func (s *SSHExecutor) DatabaseConnection(ctx context.Context) (*DatabaseConnection, error) {
	conn := defaultDatabaseConnection()

	databaseURL := s.env["DATABASE_URL"]

	if databaseURL == "" {
		var err error
		databaseURL, err = s.remoteDatabaseURL(ctx)
		if err != nil {
			return nil, err
		}
	}

	if databaseURL != "" {
		if err := applyDatabaseURL(conn, databaseURL); err != nil {
			return nil, err
		}
	}

	conn.tunneledNet = s.registerSSHDialer()

	return conn, nil
}

// remoteDatabaseURL reads the remote Symfony env files (.env.dist < .env <
// .env.local, matching envfile.ReadValue precedence) over ssh and extracts
// DATABASE_URL. Concatenating in precedence order keeps the last occurrence
// winning, mirroring godotenv.Read.
func (s *SSHExecutor) remoteDatabaseURL(ctx context.Context) (string, error) {
	remoteCmd := "cd " + shellQuoteArg(s.remoteDir()) +
		` && for f in .env.dist .env .env.local; do if [ -f "$f" ]; then cat "$f"; fi; done`

	sshArgs := append(s.sshArgs(), "-T", s.target(), remoteCmd)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	logCmd(ctx, cmd)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not read remote env files via ssh: %w\n%s", err, stderr.String())
	}

	values, err := godotenv.Parse(strings.NewReader(stdout.String()))
	if err != nil {
		return "", fmt.Errorf("could not parse remote env files: %w", err)
	}

	return values["DATABASE_URL"], nil
}

var sshDialerCounter atomic.Int64

// registerSSHDialer registers a mysql driver network that dials through the
// SSH connection and returns its name for DatabaseConnection.tunneledNet.
func (s *SSHExecutor) registerSSHDialer() string {
	name := fmt.Sprintf("shopware-cli-ssh-%d", sshDialerCounter.Add(1))

	sshArgs := append(s.sshArgs(), "-T")
	target := s.target()

	mysql.RegisterDialContext(name, func(ctx context.Context, addr string) (net.Conn, error) {
		return dialViaSSH(ctx, sshArgs, target, addr)
	})

	return name
}

// dialViaSSH dials addr (host:port in the remote network) through
// `ssh -W addr target`, returning the ssh session's stdin/stdout as a
// net.Conn. The process is intentionally not bound to ctx: database/sql
// cancels the dial context once the connection is established, which would
// kill a healthy tunnel. The ssh process dies when the connection is closed.
func dialViaSSH(_ context.Context, sshArgs []string, target, addr string) (net.Conn, error) {
	args := make([]string, 0, len(sshArgs)+3)
	args = append(args, sshArgs...)
	args = append(args, "-W", addr, target)

	cmd := exec.Command("ssh", args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ssh tunnel: %w", err)
	}

	return &sshTunnelConn{addr: addr, stdin: stdin, stdout: stdout, cmd: cmd, stderr: &stderr}, nil
}

// sshTunnelConn adapts an `ssh -W` process to net.Conn.
type sshTunnelConn struct {
	addr   string
	stdin  io.WriteCloser
	stdout io.ReadCloser
	cmd    *exec.Cmd
	stderr *strings.Builder
}

func (c *sshTunnelConn) Read(b []byte) (int, error) {
	n, err := c.stdout.Read(b)
	if err != nil && c.stderr.Len() > 0 {
		return n, fmt.Errorf("%w: %s", err, strings.TrimSpace(c.stderr.String()))
	}

	return n, err
}

func (c *sshTunnelConn) Write(b []byte) (int, error) {
	return c.stdin.Write(b)
}

func (c *sshTunnelConn) Close() error {
	_ = c.stdin.Close()
	_ = c.stdout.Close()

	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}

	_ = c.cmd.Wait()

	return nil
}

func (c *sshTunnelConn) LocalAddr() net.Addr  { return sshAddr("127.0.0.1:0") }
func (c *sshTunnelConn) RemoteAddr() net.Addr { return sshAddr(c.addr) }

// Deadlines are no-ops: go-sql-driver only sets them when timeout parameters
// are configured, and the underlying pipe does not support them.
func (c *sshTunnelConn) SetDeadline(_ time.Time) error      { return nil }
func (c *sshTunnelConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *sshTunnelConn) SetWriteDeadline(_ time.Time) error { return nil }

type sshAddr string

func (a sshAddr) Network() string { return "tcp" }
func (a sshAddr) String() string  { return string(a) }

// shellQuoteArg quotes s for POSIX sh only when it contains characters
// outside a conservative safe set, keeping common arguments readable.
func shellQuoteArg(s string) string {
	if s == "" {
		return "''"
	}

	const safeChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-"
	if strings.IndexFunc(s, func(r rune) bool { return !strings.ContainsRune(safeChars, r) }) == -1 {
		return s
	}

	return shellSingleQuote(s)
}
