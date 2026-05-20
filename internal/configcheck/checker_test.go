package configcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/symfonyconfig"
)

// makeConfig builds a Config in-memory without touching the filesystem.
func makeConfig(data map[string]any, env map[string]string) *symfonyconfig.Config {
	if env == nil {
		env = map[string]string{}
	}
	if data == nil {
		data = map[string]any{}
	}
	return &symfonyconfig.Config{Data: data, EnvVars: env, Env: "prod"}
}

func resultIDs(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func TestAdminWorkerCheck(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{
			"admin_worker": map[string]any{
				"enable_admin_worker": true,
			},
		},
	}, nil)
	results := AdminWorkerCheck{}.Run(cfg)
	require.Len(t, results, 1)
	assert.Equal(t, "admin-worker-enabled", results[0].ID)
	assert.Equal(t, SeverityWarning, results[0].Severity)
}

func TestAdminWorkerCheck_DisabledIsClean(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{
			"admin_worker": map[string]any{
				"enable_admin_worker": false,
			},
		},
	}, nil)
	assert.Empty(t, AdminWorkerCheck{}.Run(cfg))
}

func TestIncrementStorageCheck_FlagsMysql(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{
			"increment": map[string]any{
				"user_activity": map[string]any{"type": "mysql"},
				"message_queue": map[string]any{"type": "redis"},
			},
		},
	}, nil)
	results := IncrementStorageCheck{}.Run(cfg)
	assert.ElementsMatch(t, []string{"increment-storage-mysql-user_activity"}, resultIDs(results))
}

func TestFineGrainedCachingCheck_AllThree(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{
			"cache": map[string]any{
				"tagging": map[string]any{
					"each_config":       true,
					"each_snippet":      true,
					"each_theme_config": false,
				},
			},
		},
	}, nil)
	results := FineGrainedCachingCheck{}.Run(cfg)
	assert.ElementsMatch(t, []string{
		"fine-grained-caching-each_config",
		"fine-grained-caching-each_snippet",
	}, resultIDs(results))
}

func TestDisabledMailUpdatesCheck(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{
			"mail": map[string]any{"update_mail_variables_on_send": true},
		},
	}, nil)
	require.Len(t, DisabledMailUpdatesCheck{}.Run(cfg), 1)
}

func TestAppUrlCheckDisabledCheck_NotSetTriggers(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{})
	require.Len(t, AppUrlCheckDisabledCheck{}.Run(cfg), 1)
}

func TestAppUrlCheckDisabledCheck_TruthySilenced(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{"APP_URL_CHECK_DISABLED": "1"})
	assert.Empty(t, AppUrlCheckDisabledCheck{}.Run(cfg))
}

func TestMessengerAutoSetupCheck_DefaultsToTrue(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{
		"MESSENGER_TRANSPORT_DSN": "redis://redis:6379",
	})
	results := MessengerAutoSetupCheck{}.Run(cfg)
	require.Len(t, results, 1)
	assert.Contains(t, results[0].ID, "messenger_transport_dsn")
}

func TestMessengerAutoSetupCheck_ExplicitlyDisabled(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{
		"MESSENGER_TRANSPORT_DSN": "redis://redis:6379?auto_setup=false",
	})
	assert.Empty(t, MessengerAutoSetupCheck{}.Run(cfg))
}

func TestMessengerTransportCheck_DoctrineFlagsWarning(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{
		"MESSENGER_TRANSPORT_DSN": "doctrine://default",
	})
	results := MessengerTransportCheck{}.Run(cfg)
	require.Len(t, results, 1)
	assert.Equal(t, SeverityWarning, results[0].Severity)
}

func TestMessengerTransportCheck_SyncIsError(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{
		"MESSENGER_TRANSPORT_DSN": "sync://",
	})
	results := MessengerTransportCheck{}.Run(cfg)
	require.Len(t, results, 1)
	assert.Equal(t, SeverityError, results[0].Severity)
}

func TestMessengerTransportCheck_RedisIsClean(t *testing.T) {
	cfg := makeConfig(nil, map[string]string{
		"MESSENGER_TRANSPORT_DSN": "redis://redis:6379",
	})
	assert.Empty(t, MessengerTransportCheck{}.Run(cfg))
}

func TestCacheCompressionCheck_GzipFlagged(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{"cache": map[string]any{"cache_compression_method": "gzip"}},
	}, nil)
	require.Len(t, CacheCompressionCheck{}.Run(cfg), 1)
}

func TestCacheCompressionCheck_ZstdClean(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{"cache": map[string]any{"cache_compression_method": "zstd"}},
	}, nil)
	assert.Empty(t, CacheCompressionCheck{}.Run(cfg))
}

func TestProductStreamIndexingCheck(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{"product_stream": map[string]any{"indexing": true}},
	}, nil)
	require.Len(t, ProductStreamIndexingCheck{}.Run(cfg), 1)
}

func TestElasticsearchCheck_DisabledTriggers(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{"elasticsearch": map[string]any{"enabled": false}},
	}, nil)
	require.Len(t, ElasticsearchCheck{}.Run(cfg), 1)
}

func TestElasticsearchCheck_EnabledClean(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{"elasticsearch": map[string]any{"enabled": true}},
	}, nil)
	assert.Empty(t, ElasticsearchCheck{}.Run(cfg))
}

func TestMonologHandlerLevelCheck(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"monolog": map[string]any{
			"handlers": map[string]any{
				"main": map[string]any{
					"type":  "stream",
					"level": "debug",
				},
				"prod_only": map[string]any{
					"type":  "stream",
					"level": "warning",
				},
			},
		},
	}, nil)
	results := MonologHandlerLevelCheck{}.Run(cfg)
	assert.ElementsMatch(t, []string{"monolog-handler-level-main"}, resultIDs(results))
}

func TestMonologHandlerLevelCheck_ResolvesEnvPlaceholder(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"monolog": map[string]any{
			"handlers": map[string]any{
				"main": map[string]any{
					"type":  "stream",
					"level": "%env(LOG_LEVEL)%",
				},
			},
		},
	}, map[string]string{"LOG_LEVEL": "debug"})
	require.Len(t, MonologHandlerLevelCheck{}.Run(cfg), 1)
}

func TestRun_SortsBySeverityThenID(t *testing.T) {
	cfg := makeConfig(map[string]any{
		"shopware": map[string]any{
			"admin_worker": map[string]any{"enable_admin_worker": true},
			"mail":         map[string]any{"update_mail_variables_on_send": true},
		},
	}, map[string]string{
		"MESSENGER_TRANSPORT_DSN": "sync://",
	})
	results := Run(cfg, Default())
	require.NotEmpty(t, results)

	// First result must be the highest-severity finding.
	assert.Equal(t, SeverityError, results[0].Severity)

	// Severity descending overall.
	for i := 1; i < len(results); i++ {
		assert.LessOrEqual(t, int(results[i].Severity), int(results[i-1].Severity),
			"severity should be non-increasing across results")
	}
}
