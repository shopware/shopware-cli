package configcheck

import "github.com/shopware/shopware-cli/internal/symfonyconfig"

// CacheCompressionCheck flags shopware.cache.cache_compression_method = gzip.
// zstd is significantly faster at the same compression ratio and is the
// recommended setting on 6.7+.
type CacheCompressionCheck struct{}

func (CacheCompressionCheck) ID() string { return "cache-compression-gzip" }

// compressionConstraint mirrors FroshTools: the compression_method config
// landed in 6.6.4.0 and 6.7.1.0 switched the default to zstd, so the
// recommendation only fires inside that window.
const compressionConstraint = ">=6.6.4.0 <6.7.1.0"

func (CacheCompressionCheck) Run(cfg *symfonyconfig.Config) []Result {
	if !shopwareVersionMatches(cfg, compressionConstraint) {
		return nil
	}
	const path = "shopware.cache.cache_compression_method"
	v, ok := cfg.GetString(path)
	if !ok || v == "" || v == "zstd" {
		return nil
	}
	return []Result{{
		ID:       "cache-compression-gzip",
		Title:    "Cache compression method is gzip",
		Message:  "shopware.cache.cache_compression_method is set to gzip. zstd compresses faster at the same ratio; switch to zstd if the PHP zstd extension is available.",
		Severity: SeverityInfo,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
		Path:     path,
	}}
}

// CartCompressionCheck mirrors CacheCompressionCheck for cart payloads.
type CartCompressionCheck struct{}

func (CartCompressionCheck) ID() string { return "cart-compression-gzip" }

func (CartCompressionCheck) Run(cfg *symfonyconfig.Config) []Result {
	if !shopwareVersionMatches(cfg, compressionConstraint) {
		return nil
	}
	const path = "shopware.cart.compression_method"
	v, ok := cfg.GetString(path)
	if !ok || v == "" || v == "zstd" {
		return nil
	}
	return []Result{{
		ID:       "cart-compression-gzip",
		Title:    "Cart compression method is gzip",
		Message:  "shopware.cart.compression_method is set to gzip. zstd is faster at the same ratio — switch to zstd when the PHP zstd extension is installed.",
		Severity: SeverityInfo,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
		Path:     path,
	}}
}
