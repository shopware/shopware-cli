package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestGenerateComposeFile(t *testing.T) {
	t.Parallel()

	t.Run("base only", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "web:")
		assert.Contains(t, compose, "database:")
		assert.Contains(t, compose, "adminer:")
		assert.Contains(t, compose, "mailer:")
		assert.Contains(t, compose, "db-data:")
		assert.Contains(t, compose, "ghcr.io/shopware/docker-dev:php8.3-node22-caddy")
		assert.Contains(t, compose, "mariadb:11.8")
		assert.Contains(t, compose, "mailpit")
		assert.NotContains(t, compose, "lavinmq")
		assert.NotContains(t, compose, "opensearch")
		assert.NotContains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.NotContains(t, compose, "OPENSEARCH_URL")
		assert.NotContains(t, compose, "PHP_PROFILER")
	})

	t.Run("with amqp", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "lavinmq:")
		assert.Contains(t, compose, "cloudamqp/lavinmq")
		assert.Contains(t, compose, "lavinmq-data:")
		assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.Contains(t, compose, "15672:15672")
		assert.Contains(t, compose, "5672:5672")
		assert.NotContains(t, compose, "opensearch")
	})

	t.Run("with elasticsearch", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "opensearch:")
		assert.Contains(t, compose, "opensearchproject/opensearch:2")
		assert.Contains(t, compose, "opensearch-data:")
		assert.Contains(t, compose, "OPENSEARCH_URL")
		assert.Contains(t, compose, "SHOPWARE_ES_ENABLED")
		assert.Contains(t, compose, "9200:9200")
		assert.NotContains(t, compose, "lavinmq")
	})

	t.Run("custom php version", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{PHPVersion: "8.2"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "ghcr.io/shopware/docker-dev:php8.2-node22-caddy")
		assert.NotContains(t, compose, "php8.3")
	})

	t.Run("custom node version", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{NodeVersion: "24"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "ghcr.io/shopware/docker-dev:php8.3-node24-caddy")
		assert.NotContains(t, compose, "node22")
	})

	t.Run("with php profiler", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{PHPProfiler: "xdebug"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "PHP_PROFILER: xdebug")
		assert.Contains(t, compose, "XDEBUG_MODE: debug")
		assert.Contains(t, compose, "XDEBUG_CONFIG: client_host=host.docker.internal")
	})

	t.Run("with blackfire profiler and credentials", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{
			PHPProfiler:          "blackfire",
			BlackfireServerID:    "my-server-id",
			BlackfireServerToken: "my-server-token",
		})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "PHP_PROFILER: blackfire")
		assert.Contains(t, compose, "blackfire:")
		assert.Contains(t, compose, "blackfire/blackfire:2")
		assert.Contains(t, compose, "BLACKFIRE_SERVER_ID: my-server-id")
		assert.Contains(t, compose, "BLACKFIRE_SERVER_TOKEN: my-server-token")
		assert.NotContains(t, compose, "XDEBUG_MODE")
		assert.NotContains(t, compose, "XDEBUG_CONFIG")
	})

	t.Run("blackfire without credentials skips container", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{PHPProfiler: "blackfire"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "PHP_PROFILER: blackfire")
		assert.NotContains(t, compose, "blackfire/blackfire:2")
		assert.NotContains(t, compose, "BLACKFIRE_SERVER_ID")
	})

	t.Run("with tideways profiler and api key", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{
			PHPProfiler:    "tideways",
			TidewaysAPIKey: "my-api-key",
		})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "PHP_PROFILER: tideways")
		assert.Contains(t, compose, "TIDEWAYS_APIKEY: my-api-key")
		assert.Contains(t, compose, "tideways-daemon:")
		assert.Contains(t, compose, "ghcr.io/tideways/daemon")
		assert.NotContains(t, compose, "XDEBUG_MODE")
		assert.NotContains(t, compose, "blackfire/blackfire")
	})

	t.Run("tideways without api key skips container", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{PHPProfiler: "tideways"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "PHP_PROFILER: tideways")
		assert.NotContains(t, compose, "ghcr.io/tideways/daemon")
		assert.NotContains(t, compose, "TIDEWAYS_APIKEY")
	})

	t.Run("without php profiler", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.NotContains(t, compose, "PHP_PROFILER")
		assert.NotContains(t, compose, "XDEBUG_MODE")
		assert.NotContains(t, compose, "XDEBUG_CONFIG")
	})

	t.Run("with all optional services", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
				{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "web:")
		assert.Contains(t, compose, "database:")
		assert.Contains(t, compose, "adminer:")
		assert.Contains(t, compose, "mailer:")
		assert.Contains(t, compose, "lavinmq:")
		assert.Contains(t, compose, "opensearch:")
		assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.Contains(t, compose, "OPENSEARCH_URL")
	})
}
