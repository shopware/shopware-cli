package symfonyconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func newConfigForResolve(env, params map[string]any) *Config {
	envVars := map[string]string{}
	for k, v := range env {
		envVars[k] = v.(string)
	}
	return &Config{
		Data:    map[string]any{"parameters": params},
		EnvVars: envVars,
	}
}

func TestResolve_PassthroughWithoutPlaceholders(t *testing.T) {
	c := &Config{Data: map[string]any{}, EnvVars: map[string]string{}}
	assert.Equal(t, "hello", c.Resolve("hello"))
}

func TestResolve_EnvVariable(t *testing.T) {
	c := newConfigForResolve(map[string]any{"FOO": "bar"}, nil)
	assert.Equal(t, "bar", c.Resolve("%env(FOO)%"))
}

func TestResolve_EnvWithDefault(t *testing.T) {
	c := newConfigForResolve(map[string]any{}, nil)
	// Symfony's env() processor splits arguments on ':', so the default
	// value cannot itself contain a colon. Real configs usually use a
	// parameter reference or an empty default in that case.
	assert.Equal(t, "fallback", c.Resolve("%env(default:fallback:MESSENGER_TRANSPORT_DSN)%"))
}

func TestResolve_EnvWithEmptyDefault(t *testing.T) {
	c := newConfigForResolve(map[string]any{}, nil)
	assert.Equal(t, "", c.Resolve("%env(default::MESSENGER_TRANSPORT_DSN)%"))
}

func TestResolve_EnvProcessorsArePassedThrough(t *testing.T) {
	// We don't cast types; rule code does its own casting. The raw value
	// should come back regardless of processor prefix.
	c := newConfigForResolve(map[string]any{"APP_URL_CHECK_DISABLED": "1"}, nil)
	assert.Equal(t, "1", c.Resolve("%env(bool:APP_URL_CHECK_DISABLED)%"))
}

func TestResolve_ParameterReference(t *testing.T) {
	c := newConfigForResolve(nil, map[string]any{"shopware.cart.compress": "true"})
	assert.Equal(t, "true", c.Resolve("%shopware.cart.compress%"))
}

func TestResolve_ParameterReferenceRecursive(t *testing.T) {
	c := newConfigForResolve(
		map[string]any{"REDIS_URL": "redis://cache"},
		map[string]any{"my_app.redis": "%env(REDIS_URL)%"},
	)
	assert.Equal(t, "redis://cache", c.Resolve("%my_app.redis%"))
}

func TestResolve_UnknownPlaceholderKeptVerbatim(t *testing.T) {
	c := newConfigForResolve(nil, nil)
	assert.Equal(t, "%missing%", c.Resolve("%missing%"))
}

func TestResolve_EscapedPercent(t *testing.T) {
	c := newConfigForResolve(map[string]any{"FOO": "bar"}, nil)
	assert.Equal(t, "100% bar", c.Resolve("100%% %env(FOO)%"))
}

func TestResolve_AdjacentPlaceholders(t *testing.T) {
	c := newConfigForResolve(map[string]any{"A": "x", "B": "y"}, nil)
	assert.Equal(t, "x-y", c.Resolve("%env(A)%-%env(B)%"))
}
