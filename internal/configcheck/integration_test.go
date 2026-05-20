package configcheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// TestRun_AgainstRealProjectLayout exercises the full pipeline: load
// config/packages + env-specific overrides + .env files, then run the
// default rule set against the resolved configuration.
func TestRun_AgainstRealProjectLayout(t *testing.T) {
	dir := t.TempDir()

	mkdir := func(p ...string) {
		require.NoError(t, os.MkdirAll(filepath.Join(append([]string{dir}, p...)...), 0o755))
	}
	write := func(content string, p ...string) {
		require.NoError(t, os.WriteFile(filepath.Join(append([]string{dir}, p...)...), []byte(content), 0o644))
	}

	mkdir("config", "packages", "prod")

	// Base: a couple of intentionally bad settings.
	write(`shopware:
    admin_worker:
        enable_admin_worker: true
    increment:
        user_activity:
            type: mysql
        message_queue:
            type: array
    mail:
        update_mail_variables_on_send: true
    cache:
        cache_compression_method: gzip
    elasticsearch:
        enabled: false
`, "config", "packages", "shopware.yaml")

	// Prod fixes the admin worker.
	write(`shopware:
    admin_worker:
        enable_admin_worker: false
`, "config", "packages", "prod", "shopware.yaml")

	// Bad messenger DSN in env.
	write("MESSENGER_TRANSPORT_DSN=doctrine://default\n", ".env")

	t.Setenv("MESSENGER_TRANSPORT_DSN", "")
	t.Setenv("APP_URL_CHECK_DISABLED", "")
	t.Setenv("APP_ENV", "")

	cfg, err := symfonyconfig.Load(dir, symfonyconfig.Options{Env: "prod"})
	require.NoError(t, err)

	results := Run(cfg, Default())
	ids := resultIDs(results)

	// Admin worker disabled by prod override -> no finding.
	assert.NotContains(t, ids, "admin-worker-enabled")

	// Bad MySQL increment storage still present from base.
	assert.Contains(t, ids, "increment-storage-mysql-user_activity")
	assert.NotContains(t, ids, "increment-storage-mysql-message_queue")

	// Other base findings.
	assert.Contains(t, ids, "mail-update-variables-on-send")
	assert.Contains(t, ids, "cache-compression-gzip")
	assert.Contains(t, ids, "elasticsearch-disabled")

	// Messenger checks from .env.
	assert.Contains(t, ids, "messenger-transport-doctrine-messenger_transport_dsn")
	assert.Contains(t, ids, "app-url-check-not-disabled")
}
