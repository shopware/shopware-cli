package executor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func testSSHExecutor() *SSHExecutor {
	return &SSHExecutor{host: "shop.example.com", user: "deploy", directory: "/var/www/shop", projectRoot: "/project"}
}

func TestNewSSHExecutor(t *testing.T) {
	cfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH:  &shop.EnvironmentSSHConfig{Host: "shop.example.com", User: "deploy", Directory: "/var/www/shop"},
	}

	e, err := New("/project", cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "ssh", e.Type())
}

func TestNewSSHExecutorValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *shop.EnvironmentConfig
		wantErr string
	}{
		{
			name:    "missing ssh section",
			cfg:     &shop.EnvironmentConfig{Type: "ssh"},
			wantErr: "requires an ssh section",
		},
		{
			name:    "missing host",
			cfg:     &shop.EnvironmentConfig{Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Directory: "/var/www/shop"}},
			wantErr: "requires ssh.host",
		},
		{
			name:    "missing directory",
			cfg:     &shop.EnvironmentConfig{Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Host: "shop.example.com"}},
			wantErr: "requires ssh.directory",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New("/project", tc.cfg, &shop.Config{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

// lastSSHShell returns the remote shell snippet (the final ssh argument) of a
// built process.
func lastSSHShell(t *testing.T, p *Process) string {
	t.Helper()

	require.Equal(t, "ssh", filepath.Base(p.Cmd.Path))
	require.NotEmpty(t, p.Cmd.Args)

	return p.Cmd.Args[len(p.Cmd.Args)-1]
}

func TestSSHExecutorConsoleCommand(t *testing.T) {
	e := testSSHExecutor()

	p := e.ConsoleCommand(t.Context(), "cache:clear")

	wantArgs := []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=" + e.controlPath(),
		"-o", "ControlPersist=10m",
		"-o", "LogLevel=ERROR",
		"-T",
		"deploy@shop.example.com",
		"cd /var/www/shop && php bin/console cache:clear",
	}
	assert.Equal(t, append([]string{"ssh"}, wantArgs...), p.Cmd.Args)
}

func TestSSHExecutorPipeliningArgs(t *testing.T) {
	e := testSSHExecutor()

	for _, p := range []*Process{
		e.ConsoleCommand(t.Context(), "cache:clear"),
		e.ComposerCommand(t.Context(), "install"),
		e.PHPCommand(t.Context(), "-v"),
		e.NPMCommand(t.Context(), "run", "dev"),
	} {
		args := strings.Join(p.Cmd.Args, " ")
		assert.Contains(t, args, "ControlMaster=auto")
		assert.Contains(t, args, "ControlPath=")
		assert.Contains(t, args, "ControlPersist=")
	}
}

func TestSSHExecutorControlPathStaysShort(t *testing.T) {
	e := testSSHExecutor()

	// ssh appends a random suffix (~17 chars) while binding the socket, and
	// sun_path is limited to 104 bytes on macOS.
	assert.LessOrEqual(t, len(e.controlPath())+17, 104, "control socket path plus ssh suffix must fit into sun_path")
}

func TestSSHExecutorTargetVariants(t *testing.T) {
	e := &SSHExecutor{host: "shop.example.com", directory: "/var/www/shop"}

	p := e.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "shop.example.com", "without user the bare host is the target")
	assert.NotContains(t, p.Cmd.Args, "-p", "default port 22 must not be passed")

	e = &SSHExecutor{host: "shop.example.com", user: "deploy", port: 2222, directory: "/var/www/shop", identityFile: "/keys/id_ed25519"}

	p = e.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "deploy@shop.example.com")
	assert.Contains(t, p.Cmd.Args, "-p")
	assert.Contains(t, p.Cmd.Args, "2222")
	assert.Contains(t, p.Cmd.Args, "-i")
	assert.Contains(t, p.Cmd.Args, "/keys/id_ed25519")
}

func TestSSHExecutorCommandVariants(t *testing.T) {
	e := testSSHExecutor()

	tests := []struct {
		name string
		p    *Process
		want string
	}{
		{"composer", e.ComposerCommand(t.Context(), "install", "--no-dev"), "cd /var/www/shop && composer install --no-dev"},
		{"php", e.PHPCommand(t.Context(), "-r", "echo 1;"), "cd /var/www/shop && php -r 'echo 1;'"},
		{"npm", e.NPMCommand(t.Context(), "run", "build"), "cd /var/www/shop && npm run build"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, lastSSHShell(t, tc.p))
		})
	}
}

func TestSSHExecutorQuotesArguments(t *testing.T) {
	e := testSSHExecutor()

	p := e.ConsoleCommand(t.Context(), "snippet:import", "--include-queries='ALTER TABLE'")

	assert.Equal(t, `cd /var/www/shop && php bin/console snippet:import '--include-queries='\''ALTER TABLE'\'''`, lastSSHShell(t, p))
}

func TestSSHExecutorEnvIsSortedAndQuoted(t *testing.T) {
	e := testSSHExecutor().WithEnv(map[string]string{
		"ZULU":  "last",
		"APP":   "with space",
		"FIRST": "1",
	})

	p := e.PHPCommand(t.Context(), "-v")

	shell := lastSSHShell(t, p)
	assert.Contains(t, shell, "APP='with space' FIRST=1 ZULU=last php -v")
	assert.Less(t, strings.Index(shell, "APP="), strings.Index(shell, "FIRST="))
	assert.Less(t, strings.Index(shell, "FIRST="), strings.Index(shell, "ZULU="))
}

func TestSSHExecutorWithEnvNormalizesProjectRoots(t *testing.T) {
	e := testSSHExecutor().WithEnv(map[string]string{
		"PROJECT_ROOT": "/project",
		"ADMIN_ROOT":   "/project/vendor/shopware/administration/Resources/app/administration",
		"UNRELATED":    "/elsewhere",
	})

	p := e.PHPCommand(t.Context(), "-v")
	shell := lastSSHShell(t, p)

	assert.Contains(t, shell, "PROJECT_ROOT=/var/www/shop")
	assert.Contains(t, shell, "ADMIN_ROOT=/var/www/shop/vendor/shopware/administration/Resources/app/administration")
	assert.Contains(t, shell, "UNRELATED=/elsewhere")
}

func TestSSHExecutorWithRelDir(t *testing.T) {
	e := testSSHExecutor().WithRelDir("custom/plugins/SwagTest")

	p := e.ComposerCommand(t.Context(), "dump-autoload")

	assert.Equal(t, "cd /var/www/shop/custom/plugins/SwagTest && composer dump-autoload", lastSSHShell(t, p))
}

func TestSSHExecutorTTY(t *testing.T) {
	e := testSSHExecutor()

	p := e.ConsoleCommand(WithTTY(t.Context()), "cache:clear")
	assert.Contains(t, p.Cmd.Args, "-t", "WithTTY must request a TTY")
	assert.NotContains(t, p.Cmd.Args, "-T")

	p = e.ConsoleCommand(t.Context(), "cache:clear")
	assert.Contains(t, p.Cmd.Args, "-T", "TTY must be disabled by default")
	assert.NotContains(t, p.Cmd.Args, "-t")
}

func TestSSHExecutorNormalizePath(t *testing.T) {
	e := testSSHExecutor()

	assert.Equal(t, "/var/www/shop/custom/plugins/Foo", e.NormalizePath("/project/custom/plugins/Foo"))
	// Mirrors the Docker executor: relative paths escaping the project root
	// are joined into the remote project directory as-is.
	assert.Equal(t, "/var/www/outside/project", e.NormalizePath("/outside/project"))

	noRoot := &SSHExecutor{host: "shop.example.com", directory: "/var/www/shop"}
	assert.Equal(t, "/project/custom/plugins/Foo", noRoot.NormalizePath("/project/custom/plugins/Foo"))
}

func TestSSHExecutorEnvironmentLifecycleUnsupported(t *testing.T) {
	e := testSSHExecutor()

	assert.ErrorIs(t, e.StartEnvironment(t.Context()), ErrNotSupported)
	assert.ErrorIs(t, e.StopEnvironment(t.Context(), StopOptions{}), ErrNotSupported)

	running, err := e.EnvironmentStatus(t.Context())
	assert.ErrorIs(t, err, ErrNotSupported)
	assert.False(t, running)
}

// writeRecordingSSH installs an ssh stub on PATH that records its arguments
// into argsFile and prints the content of outputFile, simulating a remote
// command result. When failPattern is non-empty, invocations containing it
// exit non-zero, simulating a failing remote command. When socketFile is
// non-empty, invocations querying the MySQL socket print its content
// instead.
func writeRecordingSSH(t *testing.T, argsFile, outputFile, failPattern, socketFile string) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake ssh binary requires a POSIX shell")
	}

	shPath, err := exec.LookPath("sh")
	require.NoError(t, err)

	failSnippet := ""
	if failPattern != "" {
		failSnippet = fmt.Sprintf("case \"$*\" in *%s*) exit 1 ;; esac\n", failPattern)
	}

	socketSnippet := ""
	if socketFile != "" {
		socketSnippet = fmt.Sprintf("case \"$*\" in *default_socket*) /bin/cat %q; exit 0 ;; esac\n", socketFile)
	}

	// /bin/cat by absolute path: the stub replaces PATH, so nothing else resolves.
	script := fmt.Sprintf("#!%s\nprintf '%%s\\n' \"$@\" > %q\n%s%s/bin/cat %q\n", shPath, argsFile, failSnippet, socketSnippet, outputFile)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ssh"), []byte(script), 0o755))
	t.Setenv("PATH", dir)
}

func TestSSHExecutorDatabaseConnection(t *testing.T) {
	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "args.txt")
	envFile := filepath.Join(tmp, "env.txt")

	require.NoError(t, os.WriteFile(envFile, []byte(`{"APP_ENV":"prod","DATABASE_URL":"mysql://app:secret@db.internal:3307/shopware_prod"}`), 0o644))
	writeRecordingSSH(t, argsFile, envFile, "", "")

	e := testSSHExecutor()

	conn, err := e.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "db.internal", conn.Host)
	assert.Equal(t, "3307", conn.Port)
	assert.Equal(t, "app", conn.Username)
	assert.Equal(t, "secret", conn.Password)
	assert.Equal(t, "shopware_prod", conn.Database)

	cfg := conn.MySQLConfig()
	assert.NotEqual(t, "tcp", cfg.Net, "SSH connections must be tunneled through a registered dialer network")
	assert.True(t, strings.HasPrefix(cfg.Net, "shopware-cli-ssh-"))
	assert.Equal(t, "db.internal:3307", cfg.Addr, "the dialer receives the remote address")

	recorded, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Contains(t, string(recorded), "deploy@shop.example.com")
	assert.Contains(t, string(recorded), "dump-env", "DATABASE_URL is resolved via deployment-helper dump-env")
}

func TestSSHExecutorDatabaseConnectionMissingDeploymentHelper(t *testing.T) {
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, "env.txt")

	require.NoError(t, os.WriteFile(envFile, nil, 0o644))
	writeRecordingSSH(t, filepath.Join(tmp, "args.txt"), envFile, "test -x", "")

	e := testSSHExecutor()

	_, err := e.DatabaseConnection(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "shopware-deployment-helper not found")
	assert.Contains(t, err.Error(), "composer require shopware/deployment-helper")
}

func TestSSHExecutorDatabaseConnectionOutdatedDeploymentHelper(t *testing.T) {
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, "env.txt")

	require.NoError(t, os.WriteFile(envFile, nil, 0o644))
	writeRecordingSSH(t, filepath.Join(tmp, "args.txt"), envFile, "dump-env", "")

	e := testSSHExecutor()

	_, err := e.DatabaseConnection(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "composer update shopware/deployment-helper")
}

func TestSSHExecutorDatabaseConnectionDefaults(t *testing.T) {
	tmp := t.TempDir()
	envFile := filepath.Join(tmp, "env.txt")

	require.NoError(t, os.WriteFile(envFile, []byte(`{"APP_ENV":"prod"}`), 0o644))
	writeRecordingSSH(t, filepath.Join(tmp, "args.txt"), envFile, "", "")

	e := testSSHExecutor()

	conn, err := e.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", conn.Host)
	assert.Equal(t, "3306", conn.Port)
	assert.Equal(t, "shopware", conn.Database)
	assert.NotEqual(t, "tcp", conn.MySQLConfig().Net)
}

func TestSSHExecutorDatabaseConnectionLocalhostSocket(t *testing.T) {
	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "args.txt")
	envFile := filepath.Join(tmp, "env.txt")
	socketFile := filepath.Join(tmp, "socket.txt")

	require.NoError(t, os.WriteFile(envFile, []byte(`{"DATABASE_URL":"mysql://app:secret@localhost/shopware_prod"}`), 0o644))
	require.NoError(t, os.WriteFile(socketFile, []byte("/run/mysqld/mysqld.sock\n"), 0o644))
	writeRecordingSSH(t, argsFile, envFile, "", socketFile)

	e := testSSHExecutor()

	conn, err := e.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "localhost", conn.Host)
	assert.Equal(t, "shopware_prod", conn.Database)

	socket, err := e.remoteMySQLSocket(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/run/mysqld/mysqld.sock", socket)

	recorded, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Contains(t, string(recorded), "default_socket", "the remote PHP socket configuration is queried")
}

func TestDialViaSSHUnixSocket(t *testing.T) {
	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "args.txt")
	outFile := filepath.Join(tmp, "out.txt")

	require.NoError(t, os.WriteFile(outFile, nil, 0o644))
	writeRecordingSSH(t, argsFile, outFile, "", "")

	c, err := dialViaSSH(t.Context(), []string{"-T"}, "deploy@shop.example.com", "/run/mysqld/mysqld.sock")
	require.NoError(t, err)
	defer func() { require.NoError(t, c.Close()) }()

	// Drain the connection: the stub exits after printing, which guarantees
	// it recorded its arguments before we read them.
	_, err = io.ReadAll(c)
	require.NoError(t, err)

	recorded, err := os.ReadFile(argsFile)
	require.NoError(t, err)
	assert.Contains(t, string(recorded), "-W")
	assert.Contains(t, string(recorded), "/run/mysqld/mysqld.sock")
}

func TestSSHExecutorDatabaseConnectionEnvOverride(t *testing.T) {
	e := testSSHExecutor().WithEnv(map[string]string{
		"DATABASE_URL": "mysql://root:root@127.0.0.1:3306/override",
	})

	conn, err := e.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "override", conn.Database, "executor env wins without any ssh call")
}

func TestShellQuoteArg(t *testing.T) {
	assert.Equal(t, "cache:clear", shellQuoteArg("cache:clear"))
	assert.Equal(t, "''", shellQuoteArg(""))
	assert.Equal(t, "'with space'", shellQuoteArg("with space"))
	assert.Equal(t, `'it'\''s'`, shellQuoteArg("it's"))
	assert.Equal(t, "'semi;colon'", shellQuoteArg("semi;colon"))
	assert.Equal(t, "'dollar$var'", shellQuoteArg("dollar$var"))
}
