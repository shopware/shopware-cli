package docker

import "github.com/shyim/go-composer"

// features is the set of composer.lock packages that change the generated
// compose file and the matching proxy hostnames.
type features struct {
	AMQP           bool
	Elasticsearch  bool
	RedisMessenger bool
	// S3 is set when shopware/k8s-meta is in the lock. That recipe expects
	// S3 filesystems plus Redis cache/session env.
	S3 bool
}

// needsRedis reports whether the stack includes the cache service: either
// the Redis messenger transport is present, or S3/PaaS (shopware/k8s-meta,
// which requires Redis for cache and sessions) is.
func (f features) needsRedis() bool {
	return f.RedisMessenger || f.S3
}

// featuresFromLock reads the lock packages that drive optional compose
// services. A nil lock is treated as empty (no optional infra).
func featuresFromLock(lock *composer.Lock) features {
	if lock == nil {
		return features{}
	}

	return features{
		AMQP:           lock.GetPackage("symfony/amqp-messenger") != nil,
		Elasticsearch:  lock.GetPackage("shopware/elasticsearch") != nil,
		RedisMessenger: lock.GetPackage("symfony/redis-messenger") != nil,
		S3:             lock.GetPackage("shopware/k8s-meta") != nil,
	}
}
