package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeaturesFromLock(t *testing.T) {
	t.Parallel()

	assert.Equal(t, LockFeatures{}, FeaturesFromLock(nil))
	assert.Equal(t, LockFeatures{}, FeaturesFromLock(&composer.Lock{}))

	f := FeaturesFromLock(&composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core"},
			{Name: "symfony/amqp-messenger"},
			{Name: "shopware/elasticsearch"},
			{Name: "symfony/redis-messenger"},
			{Name: "shopware/k8s-meta"},
		},
	})
	assert.Equal(t, LockFeatures{
		AMQP:           true,
		Elasticsearch:  true,
		RedisMessenger: true,
		K8sMeta:        true,
	}, f)
	assert.True(t, f.NeedsRedis())
	assert.Equal(t, []string{"lavinmq", "opensearch", "s3", "rustfs"}, f.ProxySubdomains())
}

func TestFeaturesFromLockFile(t *testing.T) {
	t.Parallel()

	assert.Equal(t, LockFeatures{}, FeaturesFromLockFile(filepath.Join(t.TempDir(), "missing.lock")))

	dir := t.TempDir()
	lockPath := filepath.Join(dir, "composer.lock")
	require.NoError(t, os.WriteFile(lockPath, []byte(`{"packages":[{"name":"shopware/k8s-meta","version":"1.0.0"},{"name":"symfony/redis-messenger","version":"v7.0.0"}]}`), 0o644))

	f := FeaturesFromLockFile(lockPath)
	assert.True(t, f.K8sMeta)
	assert.True(t, f.RedisMessenger)
	assert.True(t, f.NeedsRedis())
	assert.False(t, f.AMQP)
}

func TestLockFeaturesNeedsRedis(t *testing.T) {
	t.Parallel()

	assert.False(t, LockFeatures{}.NeedsRedis())
	assert.True(t, LockFeatures{RedisMessenger: true}.NeedsRedis())
	assert.True(t, LockFeatures{K8sMeta: true}.NeedsRedis())
	assert.Empty(t, LockFeatures{}.ProxySubdomains())
	assert.Equal(t, []string{"lavinmq"}, LockFeatures{AMQP: true}.ProxySubdomains())
}
