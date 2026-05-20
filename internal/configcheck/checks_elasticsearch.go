package configcheck

import "github.com/shopware/shopware-cli/internal/symfonyconfig"

// ElasticsearchCheck reports when Shopware's OpenSearch/Elasticsearch
// integration is disabled. Large catalogs see substantial query speedups
// from enabling it.
type ElasticsearchCheck struct{}

func (ElasticsearchCheck) ID() string { return "elasticsearch-disabled" }

func (ElasticsearchCheck) Run(cfg *symfonyconfig.Config) []Result {
	const path = "shopware.elasticsearch.enabled"
	v, ok := cfg.GetBool(path)
	if ok && v {
		return nil
	}
	return []Result{{
		ID:       "elasticsearch-disabled",
		Title:    "Elasticsearch / OpenSearch is disabled",
		Message:  "shopware.elasticsearch.enabled is not true. Enable it for sizeable catalogs to offload product search from MySQL.",
		Severity: SeverityInfo,
		DocURL:   "https://developer.shopware.com/docs/guides/hosting/infrastructure/elasticsearch/elasticsearch-setup.html",
		Path:     path,
	}}
}
