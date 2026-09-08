package executor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

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

	cfg := conn.MySQLConfig()

	assert.Equal(t, "tcp", cfg.Net)
	assert.Equal(t, "db.internal:3307", cfg.Addr)
	assert.Equal(t, "app", cfg.User)
	assert.Equal(t, "secret", cfg.Passwd)
	assert.Equal(t, "shop", cfg.DBName)
	assert.Equal(t, 0, cfg.MaxAllowedPacket, "must fetch max_allowed_packet from the server")
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
