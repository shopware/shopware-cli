package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestProfilerNeedsCredentials(t *testing.T) {
	t.Parallel()

	assert.False(t, ProfilerNeedsCredentials("none"))
	assert.False(t, ProfilerNeedsCredentials(""))
	assert.False(t, ProfilerNeedsCredentials(ProfilerXdebug))
	assert.False(t, ProfilerNeedsCredentials(ProfilerPcov))
	assert.False(t, ProfilerNeedsCredentials(ProfilerSpx))
	assert.True(t, ProfilerNeedsCredentials(ProfilerBlackfire))
	assert.True(t, ProfilerNeedsCredentials(ProfilerTideways))
}

func TestProfilerIsPaid(t *testing.T) {
	t.Parallel()

	assert.False(t, ProfilerIsPaid(""))
	assert.False(t, ProfilerIsPaid(ProfilerXdebug))
	assert.False(t, ProfilerIsPaid(ProfilerPcov))
	assert.False(t, ProfilerIsPaid(ProfilerSpx))
	assert.True(t, ProfilerIsPaid(ProfilerBlackfire))
	assert.True(t, ProfilerIsPaid(ProfilerTideways))
}

func TestWriteComposeFileEnvLocal(t *testing.T) {
	t.Parallel()

	setupProject := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		lock := `{"packages": [{"name": "shopware/core", "version": "6.6.0.0"}], "packages-dev": []}`
		require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
		return dir
	}

	t.Run("creates missing env.local", func(t *testing.T) {
		t.Parallel()
		dir := setupProject(t)

		require.NoError(t, WriteComposeFile(dir, nil))

		content, err := os.ReadFile(filepath.Join(dir, ".env.local"))
		require.NoError(t, err)
		assert.Equal(t, "APP_ENV=dev\n", string(content))
	})

	t.Run("keeps existing env.local untouched", func(t *testing.T) {
		t.Parallel()
		dir := setupProject(t)
		existing := "APP_ENV=prod\nAPP_SECRET=abc\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.local"), []byte(existing), 0o644))

		require.NoError(t, WriteComposeFile(dir, nil))

		content, err := os.ReadFile(filepath.Join(dir, ".env.local"))
		require.NoError(t, err)
		assert.Equal(t, existing, string(content))
	})
}

func TestGenerateComposeFile(t *testing.T) {
	t.Parallel()

	t.Run("base only", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		assert.Contains(t, compose, "ghcr.io/shopware/docker-dev:php8.3-node24-caddy")
		assert.Contains(t, compose, "mariadb:11.8")
		assert.Contains(t, compose, "127.0.0.1::3306")
		assert.Contains(t, compose, "mailpit")
		assert.NotContains(t, compose, "lavinmq")
		assert.NotContains(t, compose, "opensearch")
		assert.NotContains(t, compose, "redis")
		assert.NotContains(t, compose, "rustfs")
		assert.NotContains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.NotContains(t, compose, "OPENSEARCH_URL")
		assert.NotContains(t, compose, "K8S_FILESYSTEM")
		assert.NotContains(t, compose, "PHP_PROFILER")
	})

	t.Run("with amqp", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		assert.NotContains(t, compose, "redis")
		assert.NotContains(t, compose, "rustfs")
	})

	t.Run("with elasticsearch", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		assert.NotContains(t, compose, "redis")
		assert.NotContains(t, compose, "rustfs")
	})

	t.Run("with redis-messenger", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/redis-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "redis:")
		assert.Contains(t, compose, "redis:7-alpine")
		assert.Contains(t, compose, "redis-data:")
		assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.Contains(t, compose, "redis://redis:6379")
		assert.NotContains(t, compose, "127.0.0.1::6379")
		assert.NotContains(t, compose, "rustfs")
		assert.NotContains(t, compose, "K8S_FILESYSTEM")
		assert.NotContains(t, compose, "lavinmq")
	})

	t.Run("redis-messenger wins over amqp", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/redis-messenger", Version: "v7.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "lavinmq:")
		assert.Contains(t, compose, "redis:")
		assert.Contains(t, compose, "redis://redis:6379")
		assert.NotContains(t, compose, "amqp://guest:guest@lavinmq")
		assert.NotContains(t, compose, "rustfs")
	})

	t.Run("with k8s-meta", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "shopware/k8s-meta", Version: "1.0.0"},
				{Name: "symfony/redis-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "redis:")
		assert.Contains(t, compose, "redis:7-alpine")
		assert.Contains(t, compose, "redis-data:")
		assert.Contains(t, compose, "redis-cli")
		assert.NotContains(t, compose, "127.0.0.1::6379")
		assert.Contains(t, compose, "rustfs:")
		assert.Contains(t, compose, "rustfs/rustfs:latest")
		assert.Contains(t, compose, "9000:9000")
		assert.Contains(t, compose, "9001:9001")
		assert.Contains(t, compose, "rustfs-data:")
		assert.Contains(t, compose, "rustfs-init:")
		assert.Contains(t, compose, "rustfs/rc:latest")
		assert.Contains(t, compose, "shopware-private")
		assert.Contains(t, compose, "shopware-public")
		assert.Contains(t, compose, "rc anonymous set download rustfs/shopware-public")
		assert.Contains(t, compose, "service_completed_successfully")
		assert.Contains(t, compose, "K8S_FILESYSTEM_PRIVATE_BUCKET: shopware-private")
		assert.Contains(t, compose, "K8S_FILESYSTEM_PUBLIC_BUCKET: shopware-public")
		assert.Contains(t, compose, "K8S_FILESYSTEM_ENDPOINT: http://rustfs:9000")
		assert.Contains(t, compose, "K8S_FILESYSTEM_PUBLIC_URL: http://127.0.0.1:9000/shopware-public")
		assert.Contains(t, compose, "K8S_FILESYSTEM_REGION: us-east-1")
		assert.Contains(t, compose, "AWS_ACCESS_KEY_ID: shopware")
		assert.Contains(t, compose, "AWS_SECRET_ACCESS_KEY: shopware")
		assert.Contains(t, compose, "AWS_DEFAULT_REGION: us-east-1")
		assert.Contains(t, compose, "K8S_CACHE_HOST: redis")
		assert.Contains(t, compose, "K8S_CACHE_PORT:")
		assert.Contains(t, compose, "PHP_SESSION_HANDLER: redis")
		assert.Contains(t, compose, "PHP_SESSION_SAVE_PATH: tcp://redis:6379")
		assert.Contains(t, compose, "K8S_ES_NUMBER_OF_REPLICAS:")
		assert.Contains(t, compose, "K8S_ES_NUMBER_OF_SHARDS:")
		assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.Contains(t, compose, "redis://redis:6379")
		assert.NotContains(t, compose, "lavinmq")
		assert.NotContains(t, compose, "amqp://")

		// The DSN contains `&`; quoted emission must still round-trip.
		var parsed struct {
			Services map[string]struct {
				Environment map[string]string `yaml:"environment"`
			} `yaml:"services"`
		}
		require.NoError(t, yaml.Unmarshal(result, &parsed))
		assert.Equal(t, redisMessengerDSN, parsed.Services["web"].Environment["MESSENGER_TRANSPORT_DSN"])
		assert.Equal(t, "1", parsed.Services["web"].Environment["K8S_ES_NUMBER_OF_REPLICAS"])
		assert.Equal(t, "1", parsed.Services["web"].Environment["K8S_ES_NUMBER_OF_SHARDS"])
	})

	t.Run("k8s-meta without redis-messenger still adds redis and rustfs", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "shopware/k8s-meta", Version: "1.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "redis:")
		assert.Contains(t, compose, "rustfs:")
		assert.Contains(t, compose, "redis://redis:6379")
	})

	t.Run("k8s-meta messenger wins over amqp", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "shopware/k8s-meta", Version: "1.0.0"},
				{Name: "symfony/redis-messenger", Version: "v7.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "lavinmq:")
		assert.Contains(t, compose, "redis:")
		assert.Contains(t, compose, "rustfs:")
		assert.Contains(t, compose, "redis://redis:6379")
		assert.NotContains(t, compose, "amqp://guest:guest@lavinmq")
	})

	t.Run("k8s-meta dedicated worker inherits redis env", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "shopware/k8s-meta", Version: "1.0.0"},
				{Name: "symfony/redis-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{DedicatedWorker: true})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "worker:")
		assert.Contains(t, compose, "scheduler:")
		assert.Contains(t, compose, "K8S_CACHE_HOST: redis")
		assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.Contains(t, compose, "redis://redis:6379")
	})

	t.Run("custom host ports", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{
			Ports: shop.ConfigDockerPorts{
				shop.DockerPortWeb:       8005,
				shop.DockerPortMailerWeb: 9925,
			},
		})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "8005:8000")
		assert.Contains(t, compose, "9925:8025")
		assert.NotContains(t, compose, "8000:8000")
		assert.NotContains(t, compose, "8025:8025")
		// Ports without an override keep their defaults.
		assert.Contains(t, compose, "1025:1025")
		assert.Contains(t, compose, "9080:8080")
		assert.Contains(t, compose, "5173:5173")
	})

	t.Run("disabled host ports", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{
			Ports: shop.ConfigDockerPorts{
				shop.DockerPortAdminer:    shop.DockerPortDisabled,
				shop.DockerPortMailerSMTP: shop.DockerPortDisabled,
				shop.DockerPortMailerWeb:  shop.DockerPortDisabled,
				shop.DockerPortWebAlt:     shop.DockerPortDisabled,
			},
		})
		assert.NoError(t, err)

		compose := string(result)
		assert.NotContains(t, compose, "9080:8080")
		assert.NotContains(t, compose, "8080:8080")
		assert.NotContains(t, compose, "1025:1025")
		assert.NotContains(t, compose, "8025:8025")
		// Remaining web ports keep publishing.
		assert.Contains(t, compose, "8000:8000")
		assert.Contains(t, compose, "5173:5173")
		// A service whose ports are all disabled loses its ports key entirely.
		var parsed struct {
			Services map[string]struct {
				Ports []string `yaml:"ports"`
			} `yaml:"services"`
		}
		assert.NoError(t, yaml.Unmarshal(result, &parsed))
		require.Contains(t, parsed.Services, "adminer")
		assert.Empty(t, parsed.Services["adminer"].Ports)
	})

	t.Run("custom php version", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{PHPVersion: "8.2"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "ghcr.io/shopware/docker-dev:php8.2-node24-caddy")
		assert.NotContains(t, compose, "php8.3")
	})

	t.Run("with php profiler", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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

	t.Run("without dedicated worker by default", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.NotContains(t, compose, "worker:")
		assert.NotContains(t, compose, "messenger:consume")
		assert.NotContains(t, compose, "scheduler:")
		assert.NotContains(t, compose, "scheduled-task:run")
	})

	t.Run("with dedicated worker", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
				{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{DedicatedWorker: true})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "worker:")
		assert.Contains(t, compose, "messenger:consume")
		assert.Contains(t, compose, "--all")
		assert.Contains(t, compose, "scheduler:")
		assert.Contains(t, compose, "scheduled-task:run")
		// The worker reuses the web image and shares its messenger transport env.
		assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN")
		assert.Contains(t, compose, "unless-stopped")
	})

	t.Run("with all optional services", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
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

	t.Run("has no Traefik labels or shared network", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.NotContains(t, compose, "traefik")
		assert.NotContains(t, compose, "shopware-cli-proxy")
		assert.Contains(t, compose, "8000:8000")
		assert.Contains(t, compose, "9080:8080")
	})

	t.Run("emits user when set", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, &ComposeOptions{User: "1001:46"})
		assert.NoError(t, err)

		compose := string(result)
		assert.Contains(t, compose, "user:")
		assert.Contains(t, compose, "1001:46")
	})

	t.Run("no user key without User", func(t *testing.T) {
		t.Parallel()
		lock := &composer.Lock{
			Packages: []composer.LockPackage{
				{Name: "shopware/core", Version: "6.6.0.0"},
			},
		}

		result, err := GenerateComposeFile(lock, nil)
		assert.NoError(t, err)

		compose := string(result)
		assert.NotContains(t, compose, "user:")
	})
}
