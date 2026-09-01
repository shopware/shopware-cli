package docker

import "github.com/shyim/go-composer"

// LockFeatures is the set of composer.lock packages that change the generated
// compose file and the matching proxy hostnames. Detected once via
// FeaturesFromLock / FeaturesFromLockFile.
type LockFeatures struct {
	AMQP           bool
	Elasticsearch  bool
	RedisMessenger bool
	// S3 is set when shopware/k8s-meta is in the lock. That recipe expects
	// S3 filesystems (RustFS locally) plus Redis cache/session env.
	S3 bool
}

// FeaturesFromLock reads the lock packages that drive optional compose
// services. A nil lock is treated as empty (no optional infra).
func FeaturesFromLock(lock *composer.Lock) LockFeatures {
	if lock == nil {
		return LockFeatures{}
	}

	return LockFeatures{
		AMQP:           lock.GetPackage("symfony/amqp-messenger") != nil,
		Elasticsearch:  lock.GetPackage("shopware/elasticsearch") != nil,
		RedisMessenger: lock.GetPackage("symfony/redis-messenger") != nil,
		S3:             lock.GetPackage("shopware/k8s-meta") != nil,
	}
}

// FeaturesFromLockFile reads composer.lock at path. A missing or unreadable
// lock yields an empty feature set so callers can treat "no lock" as "base
// stack only".
func FeaturesFromLockFile(path string) LockFeatures {
	lock, err := composer.ReadLock(path)
	if err != nil {
		return LockFeatures{}
	}

	return FeaturesFromLock(lock)
}

// NeedsRedis reports whether the generated stack should include a Redis
// service: either the Redis messenger transport is present, or S3/PaaS
// (shopware/k8s-meta, which requires Redis for cache and sessions) is.
func (f LockFeatures) NeedsRedis() bool {
	return f.RedisMessenger || f.S3
}
