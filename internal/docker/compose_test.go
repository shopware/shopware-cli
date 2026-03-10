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

		result, err := GenerateComposeFile(lock)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "web:")
		assert.Contains(t, compose, "database:")
		assert.Contains(t, compose, "adminer:")
		assert.Contains(t, compose, "mailer:")
		assert.Contains(t, compose, "db-data:")
		assert.Contains(t, compose, "ghcr.io/shopware/docker-dev")
		assert.Contains(t, compose, "mariadb:11.8")
		assert.Contains(t, compose, "mailpit")
		assert.NotContains(t, compose, "lavinmq")
		assert.NotContains(t, compose, "opensearch")
		assert.NotContains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.NotContains(t, compose, "OPENSEARCH_URL")
	})

	t.Run("with amqp", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock)
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

		result, err := GenerateComposeFile(lock)
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

	t.Run("with all optional services", func(t *testing.T) {
		t.Parallel()
		lock := &packagist.ComposerLock{
			Packages: []packagist.ComposerLockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
				{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock)
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
