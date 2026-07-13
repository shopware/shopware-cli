package executor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestApplyDatabaseURL(t *testing.T) {
	cases := []struct {
		name     string
		url      string
		addr     string
		user     string
		password string
		dbName   string
	}{
		{
			name:     "full url",
			url:      "mysql://app:secret@db.internal:3307/shopware_prod",
			addr:     "db.internal:3307",
			user:     "app",
			password: "secret",
			dbName:   "shopware_prod",
		},
		{
			name:     "default port",
			url:      "mysql://app:secret@db.internal/shopware",
			addr:     "db.internal:3306",
			user:     "app",
			password: "secret",
			dbName:   "shopware",
		},
		{
			name:     "no password resets default",
			url:      "mysql://app@db.internal:3306/shopware",
			addr:     "db.internal:3306",
			user:     "app",
			password: "",
			dbName:   "shopware",
		},
		{
			name:     "percent encoded password",
			url:      "mysql://app:p%40ss@db.internal:3306/shopware",
			addr:     "db.internal:3306",
			user:     "app",
			password: "p@ss",
			dbName:   "shopware",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := defaultMySQLConfig()

			assert.NoError(t, applyDatabaseURL(cfg, tc.url))
			assert.Equal(t, tc.addr, cfg.Addr)
			assert.Equal(t, tc.user, cfg.User)
			assert.Equal(t, tc.password, cfg.Passwd)
			assert.Equal(t, tc.dbName, cfg.DBName)
		})
	}
}

func newDatabaseTestSSHExecutor(t *testing.T, envFiles map[string]map[string]string) *SSHExecutor {
	t.Helper()

	envCfg := &shop.EnvironmentConfig{
		Type: TypeSSH,
		SSH: &shop.EnvironmentSSH{
			Host:       "shop.example.com",
			Deployment: &shop.EnvironmentDeployment{Path: "/var/www/shopware"},
		},
	}

	executor, err := newSSHExecutor("", envCfg, &shop.Config{})
	assert.NoError(t, err)

	executor.readEnvFiles = func(_ context.Context, names ...string) (map[string]string, error) {
		merged := map[string]string{}
		for _, name := range names {
			for k, v := range envFiles[name] {
				merged[k] = v
			}
		}

		return merged, nil
	}

	return executor
}

func TestSSHDatabaseConnection(t *testing.T) {
	executor := newDatabaseTestSSHExecutor(t, map[string]map[string]string{
		".env": {"DATABASE_URL": "mysql://app:secret@10.0.0.5:3307/shop"},
	})

	cfg, err := executor.DatabaseConnection(t.Context())
	assert.NoError(t, err)

	// the address stays the server-side view, only the transport changes
	assert.Equal(t, "10.0.0.5:3307", cfg.Addr)
	assert.Equal(t, "app", cfg.User)
	assert.Equal(t, "secret", cfg.Passwd)
	assert.Equal(t, "shop", cfg.DBName)
	assert.Equal(t, sshMySQLNetwork, cfg.Net)
}

func TestSSHDatabaseConnectionAppEnvOverrides(t *testing.T) {
	executor := newDatabaseTestSSHExecutor(t, map[string]map[string]string{
		".env":            {"APP_ENV": "prod", "DATABASE_URL": "mysql://base@base:3306/base"},
		".env.local":      {"DATABASE_URL": "mysql://local@local:3306/local"},
		".env.prod.local": {"DATABASE_URL": "mysql://prod@prod:3306/prod"},
	})

	cfg, err := executor.DatabaseConnection(t.Context())
	assert.NoError(t, err)

	assert.Equal(t, "prod:3306", cfg.Addr)
	assert.Equal(t, "prod", cfg.DBName)
}

func TestSSHDatabaseConnectionWithoutDatabaseURL(t *testing.T) {
	executor := newDatabaseTestSSHExecutor(t, map[string]map[string]string{})

	cfg, err := executor.DatabaseConnection(t.Context())
	assert.NoError(t, err)

	// defaults stay, flags can override, but the transport is still ssh
	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
	assert.Equal(t, sshMySQLNetwork, cfg.Net)
}

func TestLocalDatabaseConnection(t *testing.T) {
	cfg, err := (&LocalExecutor{}).DatabaseConnection(t.Context())
	assert.NoError(t, err)

	assert.Equal(t, "tcp", cfg.Net)
	assert.Equal(t, "127.0.0.1:3306", cfg.Addr)
}
