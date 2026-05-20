package configcheck

import "github.com/shopware/shopware-cli/internal/symfonyconfig"

// ProductStreamIndexingCheck flags shopware.product_stream.indexing = true.
// The product stream indexer is expensive and on 6.6.10.5+ Shopware
// recommends disabling it for shops that don't actively use product streams.
type ProductStreamIndexingCheck struct{}

func (ProductStreamIndexingCheck) ID() string { return "product-stream-indexing" }

func (ProductStreamIndexingCheck) Run(cfg *symfonyconfig.Config) []Result {
	const path = "shopware.product_stream.indexing"
	v, ok := cfg.GetBool(path)
	if !ok || !v {
		return nil
	}
	return []Result{{
		ID:       "product-stream-indexing",
		Title:    "Product stream indexer is enabled",
		Message:  "shopware.product_stream.indexing is true. If you don't rely on product streams, set it to false to skip the indexer.",
		Severity: SeverityInfo,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/performance/performance-tweaks.html",
		Path:     path,
	}}
}
