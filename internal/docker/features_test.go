package docker

import (
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
)

func TestFeaturesFromLock(t *testing.T) {
	t.Parallel()

	assert.Equal(t, features{}, featuresFromLock(nil))
	assert.Equal(t, features{}, featuresFromLock(&composer.Lock{}))

	f := featuresFromLock(&composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core"},
			{Name: "symfony/amqp-messenger"},
			{Name: "shopware/elasticsearch"},
			{Name: "symfony/redis-messenger"},
			{Name: "shopware/k8s-meta"},
		},
	})
	assert.Equal(t, features{
		AMQP:           true,
		Elasticsearch:  true,
		RedisMessenger: true,
		S3:             true,
	}, f)
	assert.True(t, f.needsRedis())
}

func TestFeaturesNeedsRedis(t *testing.T) {
	t.Parallel()

	assert.False(t, features{}.needsRedis())
	assert.True(t, features{RedisMessenger: true}.needsRedis())
	assert.True(t, features{S3: true}.needsRedis())
}
