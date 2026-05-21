package configcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

func TestShopwareVersionMatches_UnknownVersionIsFalse(t *testing.T) {
	cfg := &symfonyconfig.Config{Data: map[string]any{}, EnvVars: map[string]string{}}
	assert.False(t, shopwareVersionMatches(cfg, ">=6.6.0.0"))
}

func TestShopwareVersionMatches_InRange(t *testing.T) {
	cfg := makeConfigWithVersion(nil, nil, "6.6.5.0")
	assert.True(t, shopwareVersionMatches(cfg, ">=6.6.4.0 <6.7.1.0"))
}

func TestShopwareVersionMatches_OutOfRange(t *testing.T) {
	cfg := makeConfigWithVersion(nil, nil, "6.7.2.0")
	assert.False(t, shopwareVersionMatches(cfg, ">=6.6.4.0 <6.7.1.0"))
}

func TestShopwareVersionMatches_PanicsOnMalformedConstraint(t *testing.T) {
	cfg := makeConfigWithVersion(nil, nil, "6.7.0.0")
	assert.Panics(t, func() {
		// Comma-with-space form is what shyim/go-version chokes on; the
		// helper must surface that loudly instead of silently passing.
		shopwareVersionMatches(cfg, ">=6.5.4.0, <6.7.0.0")
	})
}

func TestFineGrainedCachingCheck_SkippedOn67(t *testing.T) {
	cfg := makeConfigWithVersion(map[string]any{
		"shopware": map[string]any{
			"cache": map[string]any{
				"tagging": map[string]any{"each_config": true},
			},
		},
	}, nil, "6.7.0.0")
	assert.Empty(t, FineGrainedCachingCheck{}.Run(cfg),
		"each_config was removed in 6.7.0 - rule must not fire")
}

func TestFineGrainedCachingCheck_SkippedWhenVersionUnknown(t *testing.T) {
	cfg := makeConfigWithVersion(map[string]any{
		"shopware": map[string]any{
			"cache": map[string]any{
				"tagging": map[string]any{"each_config": true},
			},
		},
	}, nil, "")
	assert.Empty(t, FineGrainedCachingCheck{}.Run(cfg),
		"unknown version -> skip version-gated check")
}

func TestCacheCompressionCheck_SkippedOnOldVersion(t *testing.T) {
	cfg := makeConfigWithVersion(map[string]any{
		"shopware": map[string]any{"cache": map[string]any{"cache_compression_method": "gzip"}},
	}, nil, "6.6.3.0")
	assert.Empty(t, CacheCompressionCheck{}.Run(cfg),
		"cache_compression_method only landed in 6.6.4.0")
}

func TestCacheCompressionCheck_SkippedAfterDefaultChanged(t *testing.T) {
	cfg := makeConfigWithVersion(map[string]any{
		"shopware": map[string]any{"cache": map[string]any{"cache_compression_method": "gzip"}},
	}, nil, "6.7.1.0")
	assert.Empty(t, CacheCompressionCheck{}.Run(cfg),
		"default switched to zstd in 6.7.1.0 - upstream check stops complaining")
}

func TestProductStreamIndexingCheck_SkippedOnOldVersion(t *testing.T) {
	cfg := makeConfigWithVersion(map[string]any{
		"shopware": map[string]any{"product_stream": map[string]any{"indexing": true}},
	}, nil, "6.6.10.4")
	assert.Empty(t, ProductStreamIndexingCheck{}.Run(cfg))
}

func TestProductStreamIndexingCheck_FiresOnNewerVersion(t *testing.T) {
	cfg := makeConfigWithVersion(map[string]any{
		"shopware": map[string]any{"product_stream": map[string]any{"indexing": true}},
	}, nil, "6.7.0.0")
	assert.Len(t, ProductStreamIndexingCheck{}.Run(cfg), 1)
}
