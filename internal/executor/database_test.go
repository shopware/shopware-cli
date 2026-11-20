package executor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestDatabaseConnectionUnixSocket(t *testing.T) {
	conn := defaultDatabaseConnection()
	require.NoError(t, applyDatabaseURL(conn, "mysql://app:secret@localhost:3307/shop?unix_socket=%2Ftmp%2Fmysql%20test.sock"))
	assert.Equal(t, "/tmp/mysql test.sock", conn.Addr())
	cfg, err := conn.MySQLConfig()
	require.NoError(t, err)
	assert.Equal(t, "unix", cfg.Net)
	assert.Equal(t, conn.Addr(), cfg.Addr)
	assert.Equal(t, "app", cfg.User)
	assert.Equal(t, "secret", cfg.Passwd)
	assert.Equal(t, "shop", cfg.DBName)
	conn.tunneledNet = "ssh-test"
	cfg, err = conn.MySQLConfig()
	require.NoError(t, err)
	assert.Equal(t, "ssh-test", cfg.Net, "remote sockets must still use the SSH tunnel")
}

func TestLocalExecutorDatabaseSocketSelection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PHP socket discovery and fake PHP require Unix")
	}
	for _, tc := range []struct {
		name, url, output      string
		fail, wantError, probe bool
		wantNet, wantAddr      string
	}{
		{"localhost", "mysql://app:secret@localhost:3307/shop", "/run/mysqld/mysqld.sock\n", false, false, true, "unix", "/run/mysqld/mysqld.sock"},
		{"explicit socket", "mysql://app:secret@localhost/shop?unix_socket=/tmp/explicit.sock", "", true, false, false, "unix", "/tmp/explicit.sock"},
		{"tcp", "mysql://app:secret@127.0.0.1:3307/shop", "", true, false, false, "tcp", "127.0.0.1:3307"},
		{"empty configuration", "mysql://app:secret@localhost/shop", "", false, true, true, "", ""},
		{"PHP failure", "mysql://app:secret@localhost/shop", "", true, true, true, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			php := filepath.Join(root, "php")
			marker := filepath.Join(root, "called")
			sh, err := exec.LookPath("sh")
			require.NoError(t, err)
			script := fmt.Sprintf("#!%s\nprintf '%%s' \"$PWD:$SOCKET_TEST\" > %q\nprintf '%%b' %q\n", sh, marker, tc.output)
			if tc.fail {
				script += "exit 1\n"
			}
			require.NoError(t, os.WriteFile(php, []byte(script), 0o755))
			t.Setenv("PHP_BINARY", "")
			stubPinnedPHP(t, php, nil)
			l := &LocalExecutor{projectRoot: root, shopCfg: &shop.Config{PHPVersion: "8.5"}, env: map[string]string{"DATABASE_URL": tc.url, "SOCKET_TEST": "inherited"}}
			conn, err := l.DatabaseConnection(t.Context())
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				cfg, err := conn.MySQLConfig()
				require.NoError(t, err)
				assert.Equal(t, tc.wantNet, cfg.Net)
				assert.Equal(t, tc.wantAddr, cfg.Addr)
			}
			if tc.probe {
				out, err := os.ReadFile(marker)
				require.NoError(t, err)
				// The shell reports the physical working directory; on macOS
				// t.TempDir lives behind the /var -> /private/var symlink.
				resolvedRoot, err := filepath.EvalSymlinks(root)
				require.NoError(t, err)
				assert.Equal(t, resolvedRoot+":inherited", string(out))
			} else {
				assert.NoFileExists(t, marker)
			}
		})
	}
}

func TestMySQLSocketDial(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a Unix socket")
	}
	// Accept a real local socket connection, then close before the MySQL
	// handshake. This proves transport selection without a database or credentials.
	// Do not inherit a potentially deep test temporary root.
	//nolint:usetesting // a short private path is required even with a deep GOTMPDIR
	dir, err := os.MkdirTemp("/tmp", "sw-mysql-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	socket := filepath.Join(dir, "mysql.sock")
	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "unix", socket)
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		t.Skipf("sandbox does not permit Unix sockets: %v", err)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	accepted := make(chan struct{})
	go func() {
		c, err := listener.Accept()
		if err == nil {
			_ = c.Close()
			close(accepted)
		}
	}()
	conn := &DatabaseConnection{Host: "localhost", UnixSocket: socket, Database: "shop"}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	_, _, err = conn.Open(ctx)
	require.Error(t, err, "fake server does not speak MySQL")
	select {
	case <-accepted:
	case <-ctx.Done():
		t.Fatal("driver did not connect to the Unix socket")
	}
}

func TestDatabaseConnectionDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	conn, err := databaseConnectionFromEnv(t.TempDir(), nil)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", conn.Host)
	assert.Equal(t, "3306", conn.Port)
	assert.Equal(t, "root", conn.Username)
	assert.Equal(t, "root", conn.Password)
	assert.Equal(t, "shopware", conn.Database)
	assert.Equal(t, "127.0.0.1:3306", conn.Addr())
}

func TestDatabaseConnectionFromEnvFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	projectRoot := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=mysql://app:secret@db.example.com:3307/shop?sslmode=disable\n")

	conn, err := databaseConnectionFromEnv(projectRoot, nil)
	require.NoError(t, err)

	assert.Equal(t, "db.example.com", conn.Host)
	assert.Equal(t, "3307", conn.Port)
	assert.Equal(t, "app", conn.Username)
	assert.Equal(t, "secret", conn.Password)
	assert.Equal(t, "shop", conn.Database)
}

func TestDatabaseConnectionEnvLocalOverridesEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	projectRoot := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=mysql://a:a@one/first\n")
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env.local"), "DATABASE_URL=mysql://b:b@two/second\n")

	conn, err := databaseConnectionFromEnv(projectRoot, nil)
	require.NoError(t, err)

	assert.Equal(t, "two", conn.Host)
	assert.Equal(t, "second", conn.Database)
}

func TestDatabaseConnectionRealEnvWinsOverFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://real:env@realhost/realdb")

	projectRoot := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=mysql://file:file@filehost/filedb\n")

	conn, err := databaseConnectionFromEnv(projectRoot, nil)
	require.NoError(t, err)

	assert.Equal(t, "realhost", conn.Host)
	assert.Equal(t, "3306", conn.Port)
	assert.Equal(t, "realdb", conn.Database)
}

func TestDatabaseConnectionExecutorEnvWins(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://real:env@realhost/realdb")

	conn, err := databaseConnectionFromEnv(t.TempDir(), map[string]string{
		"DATABASE_URL": "mysql://extra:extra@extrahost/extradb",
	})
	require.NoError(t, err)

	assert.Equal(t, "extrahost", conn.Host)
	assert.Equal(t, "extradb", conn.Database)
}

func TestApplyDatabaseURL(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		expected DatabaseConnection
	}{
		{
			name:     "full url",
			url:      "mysql://user:pass@host:3307/db",
			expected: DatabaseConnection{Host: "host", Port: "3307", Username: "user", Password: "pass", Database: "db"},
		},
		{
			name:     "no port keeps default",
			url:      "mysql://user:pass@host/db",
			expected: DatabaseConnection{Host: "host", Port: "3306", Username: "user", Password: "pass", Database: "db"},
		},
		{
			name:     "user without password clears default password",
			url:      "mysql://user@host/db",
			expected: DatabaseConnection{Host: "host", Port: "3306", Username: "user", Password: "", Database: "db"},
		},
		{
			name:     "url encoded credentials",
			url:      "mysql://us%40er:p%40ss%2Fword@host/db",
			expected: DatabaseConnection{Host: "host", Port: "3306", Username: "us@er", Password: "p@ss/word", Database: "db"},
		},
		{
			name:     "query parameters ignored",
			url:      "mysql://user:pass@host/db?serverVersion=8.0&charset=utf8mb4",
			expected: DatabaseConnection{Host: "host", Port: "3306", Username: "user", Password: "pass", Database: "db"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conn := defaultDatabaseConnection()
			require.NoError(t, applyDatabaseURL(conn, tc.url))
			assert.Equal(t, tc.expected, *conn)
		})
	}
}

func TestApplyDatabaseURLInvalid(t *testing.T) {
	cases := map[string]string{
		"space in password":   "mysql://user:pa ss@host/db",
		"bare word":           "shopware",
		"missing host":        "mysql:///shopware",
		"scheme-less address": "root:root@localhost/shopware",
	}

	for name, databaseURL := range cases {
		t.Run(name, func(t *testing.T) {
			conn := defaultDatabaseConnection()
			assert.Error(t, applyDatabaseURL(conn, databaseURL))
		})
	}
}

func TestDatabaseConnectionMySQLConfig(t *testing.T) {
	conn := &DatabaseConnection{Host: "db.internal", Port: "3307", Username: "app", Password: "secret", Database: "shop"}

	cfg, err := conn.MySQLConfig()
	require.NoError(t, err)

	assert.Equal(t, "tcp", cfg.Net)
	assert.Equal(t, "db.internal:3307", cfg.Addr)
	assert.Equal(t, "app", cfg.User)
	assert.Equal(t, "secret", cfg.Passwd)
	assert.Equal(t, "shop", cfg.DBName)
	assert.Equal(t, 0, cfg.MaxAllowedPacket, "must fetch max_allowed_packet from the server")
	assert.Nil(t, cfg.TLS, "TLS is off unless configured")
}

func TestDatabaseConnectionOpenUnreachable(t *testing.T) {
	// Port 1 on loopback refuses connections immediately.
	conn := &DatabaseConnection{Host: "127.0.0.1", Port: "1", Username: "root", Password: "root", Database: "shop"}

	_, _, err := conn.Open(t.Context())
	require.Error(t, err)

	assert.Contains(t, err.Error(), `could not connect to database "shop" at 127.0.0.1:1`)
}

func TestDatabaseConnectionUnreadableEnvFile(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	projectRoot := t.TempDir()
	// A directory named .env makes the env file layer fail to read.
	if err := os.Mkdir(filepath.Join(projectRoot, ".env"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := databaseConnectionFromEnv(projectRoot, nil)
	assert.Error(t, err)
}

func TestLocalExecutorDatabaseConnection(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	projectRoot := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(projectRoot, ".env"), "DATABASE_URL=mysql://shopware:shopware@127.0.0.1:13306/dev\n")

	exec := &LocalExecutor{projectRoot: projectRoot}

	conn, err := exec.DatabaseConnection(t.Context())
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:13306", conn.Addr())
	assert.Equal(t, "shopware", conn.Username)
	assert.Equal(t, "dev", conn.Database)
}

func TestDatabaseURLMalformedSocket(t *testing.T) {
	for _, query := range []string{"unix_socket=/tmp/mysql%ZZ.sock", "unix_socket=%", "unix_socket=/valid.sock&unix_socket=%ZZ"} {
		t.Run(query, func(t *testing.T) {
			conn := defaultDatabaseConnection()
			require.Error(t, applyDatabaseURL(conn, "mysql://app:secret@127.0.0.1/shop?"+query))
		})
	}
	for _, query := range []string{"", "tls=%ZZ", "other=%ZZ&unix_socket=%2Ftmp%2Fmysql.sock"} {
		conn := defaultDatabaseConnection()
		require.NoError(t, applyDatabaseURL(conn, "mysql://app:secret@127.0.0.1/shop?"+query))
	}
}

func TestLocalDatabaseEndpointOverridesSocket(t *testing.T) {
	l := &LocalExecutor{projectRoot: t.TempDir(), env: map[string]string{"DATABASE_URL": "mysql://app:secret@localhost/shop?unix_socket=/tmp/explicit.sock"}}
	conn, err := l.DatabaseConnection(t.Context(), DatabaseEndpoint{Host: "127.0.0.1", Port: "3307"})
	require.NoError(t, err)
	cfg, err := conn.MySQLConfig()
	require.NoError(t, err)
	assert.Equal(t, "tcp", cfg.Net)
	assert.Equal(t, "127.0.0.1:3307", cfg.Addr)
	assert.Equal(t, "app", cfg.User)
}
