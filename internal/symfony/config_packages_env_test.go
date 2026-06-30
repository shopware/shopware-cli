package symfony

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolvedConfigResolvesEnvVars(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	cfg, err := pc.ResolvedConfig("dev")
	require.NoError(t, err)

	demo := cfg["env_demo"].(map[string]any)

	assert.Equal(t, "s3cr3t", demo["secret"])
	assert.Equal(t, "not-an-env-var", demo["literal"])
	assert.Equal(t, true, demo["debug"])
	assert.Equal(t, 3, demo["retry"])
	assert.Equal(t, []any{"localhost", "example.com"}, demo["hosts"])
	assert.Equal(t, map[string]any{"new_checkout": true}, demo["flags"])
}

func TestResolvedConfigUsesEnvParamDefault(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	cfg, err := pc.ResolvedConfig("dev")
	require.NoError(t, err)

	demo := cfg["env_demo"].(map[string]any)

	// REQUEST_TIMEOUT is not in .env, so the declared env(REQUEST_TIMEOUT)
	// parameter default of 45 is used and then int-cast.
	assert.Equal(t, 45, demo["timeout"])
}

func TestRawConfigKeepsEnvExpressions(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	// The non-resolving API must leave expressions intact for safe round-trips.
	value, ok, err := pc.GetConfigValue("dev", "env_demo.secret")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "%env(APP_SECRET)%", value)
}

func TestGetResolvedConfigValue(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	value, ok, err := pc.GetResolvedConfigValue("prod", "framework.cache.app")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "cache.adapter.redis", value)

	value, ok, err = pc.GetResolvedConfigValue("dev", "env_demo.retry")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 3, value)
}

func TestGetResolvedConfigValueAgreesWithResolvedConfig(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	// timeout resolves via a top-level env(REQUEST_TIMEOUT) default; the
	// single-value API must apply the same default as ResolvedConfig.
	cfg, err := pc.ResolvedConfig("dev")
	require.NoError(t, err)
	want := cfg["env_demo"].(map[string]any)["timeout"]

	got, ok, err := pc.GetResolvedConfigValue("dev", "env_demo.timeout")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, want, got)
	assert.Equal(t, 45, got)
}

func TestResolvedConfigLayersEnvironmentEnvFile(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	// .env.prod overrides REDIS_URL; dev keeps the base .env value.
	prod, err := pc.ResolvedConfig("prod")
	require.NoError(t, err)
	prodCache := prod["framework"].(map[string]any)["cache"].(map[string]any)
	assert.Equal(t, "redis://prod-host:6379", prodCache["default_redis_provider"])

	devValue, err := pc.ResolveEnvExpression("%env(REDIS_URL)%")
	require.NoError(t, err)
	assert.Equal(t, "redis://localhost:6379", devValue)
}

func TestResolveEnvExpression(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	resolved, err := pc.ResolveEnvExpression("%env(REDIS_URL)%")
	require.NoError(t, err)
	assert.Equal(t, "redis://localhost:6379", resolved)

	// A missing variable with no default resolves to an empty string.
	resolved, err = pc.ResolveEnvExpression("%env(NOT_SET)%")
	require.NoError(t, err)
	assert.Equal(t, "", resolved)
}

func TestEnv(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	env, err := pc.Env()
	require.NoError(t, err)

	assert.Equal(t, "s3cr3t", env["APP_SECRET"])
	assert.Equal(t, "redis://localhost:6379", env["REDIS_URL"])
	assert.Equal(t, "localhost,example.com", env["ALLOWED_HOSTS"])
}

func TestEnvEmptyWithoutEnvFiles(t *testing.T) {
	pc, err := NewProjectConfig(t.TempDir())
	require.NoError(t, err)

	env, err := pc.Env()
	require.NoError(t, err)
	assert.Empty(t, env)
}

func TestSplitProcessors(t *testing.T) {
	processors, varName := splitProcessors("int:default:0:PORT")
	assert.Equal(t, []string{"int", "default", "0"}, processors)
	assert.Equal(t, "PORT", varName)

	processors, varName = splitProcessors("APP_SECRET")
	assert.Nil(t, processors)
	assert.Equal(t, "APP_SECRET", varName)
}

func TestParseEnvBool(t *testing.T) {
	for _, truthy := range []string{"true", "on", "yes", "1", "5"} {
		assert.True(t, parseEnvBool(truthy), truthy)
	}
	for _, falsy := range []string{"false", "off", "no", "0", "", "nonsense"} {
		assert.False(t, parseEnvBool(falsy), falsy)
	}
}
