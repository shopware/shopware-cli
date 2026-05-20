package symfonyconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_DevMergesBaseAndEnvOverrides(t *testing.T) {
	// Make sure no MESSENGER_TRANSPORT_DSN leaks in from the host.
	t.Setenv("MESSENGER_TRANSPORT_DSN", "")
	t.Setenv("APP_ENV", "")

	cfg, err := Load("testdata/basic", Options{Env: "dev"})
	require.NoError(t, err)

	// In dev: dev/ has no shopware.yaml, so base shopware.yaml wins.
	enabled, ok := cfg.GetBool("shopware.admin_worker.enable_admin_worker")
	require.True(t, ok)
	assert.True(t, enabled, "dev should keep admin_worker enabled from base")

	// Filenames loaded - base first, then dev/, alphabetical within each.
	var loaded []string
	for _, f := range cfg.Files {
		loaded = append(loaded, filepath.Base(f.Path))
	}
	assert.Equal(t, []string{"framework.yaml", "shopware.yaml"}, loaded)
}

func TestLoad_ProdOverridesBase(t *testing.T) {
	t.Setenv("MESSENGER_TRANSPORT_DSN", "")
	t.Setenv("APP_ENV", "")

	cfg, err := Load("testdata/basic", Options{Env: "prod"})
	require.NoError(t, err)

	// prod/shopware.yaml turns off admin_worker.
	enabled, ok := cfg.GetBool("shopware.admin_worker.enable_admin_worker")
	require.True(t, ok)
	assert.False(t, enabled)

	// Nested map merge: increment.user_activity.type was overridden,
	// increment.message_queue.type should still come from base.
	userActivity, _ := cfg.GetString("shopware.increment.user_activity.type")
	assert.Equal(t, "array", userActivity)
	messageQueue, _ := cfg.GetString("shopware.increment.message_queue.type")
	assert.Equal(t, "mysql", messageQueue)

	// File order: base then prod/.
	require.Len(t, cfg.Files, 3)
	assert.Equal(t, "framework.yaml", filepath.Base(cfg.Files[0].Path))
	assert.Equal(t, "shopware.yaml", filepath.Base(cfg.Files[1].Path))
	assert.True(t, strings.Contains(cfg.Files[2].Path, filepath.Join("packages", "prod")))
}

func TestLoad_EnvFileChainProdOverridesDev(t *testing.T) {
	// Process env wins, so we explicitly clear what we test.
	t.Setenv("MESSENGER_TRANSPORT_DSN", "")
	t.Setenv("APP_ENV", "")

	cfg, err := Load("testdata/basic", Options{Env: "prod"})
	require.NoError(t, err)

	got, ok := cfg.LookupEnv("MESSENGER_TRANSPORT_DSN")
	require.True(t, ok)
	assert.Equal(t, "redis://redis:6379", got, ".env.prod should override .env")
}

func TestLoad_ResolvesEnvPlaceholder(t *testing.T) {
	t.Setenv("MESSENGER_TRANSPORT_DSN", "")
	t.Setenv("APP_ENV", "")

	cfg, err := Load("testdata/basic", Options{Env: "dev"})
	require.NoError(t, err)

	dsn, ok := cfg.GetString("framework.messenger.transports.async.dsn")
	require.True(t, ok)
	assert.Equal(t, "doctrine://default", dsn)
}

func TestLoad_ProcessEnvWinsOverDotenv(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("MESSENGER_TRANSPORT_DSN", "redis://from-process")

	cfg, err := Load("testdata/basic", Options{Env: "dev"})
	require.NoError(t, err)

	dsn, _ := cfg.GetString("framework.messenger.transports.async.dsn")
	assert.Equal(t, "redis://from-process", dsn)
}

func TestLoad_DefaultEnvFromAppEnvVar(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("MESSENGER_TRANSPORT_DSN", "")

	cfg, err := Load("testdata/basic", Options{})
	require.NoError(t, err)
	assert.Equal(t, "prod", cfg.Env)
}

func TestLoad_MissingProjectRoot(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir, Options{Env: "dev"})
	require.NoError(t, err)
	assert.Empty(t, cfg.Files)
	assert.Empty(t, cfg.Data)
}

func TestLoad_IgnoresSubdirectoriesThatArentTheEnv(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config", "packages", "test"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config", "packages", "framework.yaml"),
		[]byte("framework:\n    secret: base\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config", "packages", "test", "framework.yaml"),
		[]byte("framework:\n    secret: from-test-env\n"),
		0o644,
	))

	cfg, err := Load(dir, Options{Env: "dev"})
	require.NoError(t, err)
	v, _ := cfg.GetString("framework.secret")
	assert.Equal(t, "base", v, "test/ files must not leak into dev")
}

func TestLoad_IncludesServicesWhenRequested(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "config", "packages"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config", "services.yaml"),
		[]byte("parameters:\n    my_app.foo: from-services\n"),
		0o644,
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "config", "services_prod.yaml"),
		[]byte("parameters:\n    my_app.foo: from-services-prod\n"),
		0o644,
	))

	cfg, err := Load(dir, Options{Env: "prod", IncludeServices: true})
	require.NoError(t, err)
	// Parameter names are literal keys (they contain dots), not nested
	// paths - dotted Get doesn't apply inside the parameters: map.
	params := cfg.Data["parameters"].(map[string]any)
	assert.Equal(t, "from-services-prod", params["my_app.foo"])
}

func TestDeepMerge_ReplacesScalarsAndSequences(t *testing.T) {
	dst := map[string]any{
		"a": "old",
		"b": []any{1, 2},
		"c": map[string]any{"x": 1, "y": 2},
	}
	src := map[string]any{
		"a": "new",
		"b": []any{9},
		"c": map[string]any{"y": 99, "z": 3},
	}
	deepMerge(dst, src)

	assert.Equal(t, "new", dst["a"])
	assert.Equal(t, []any{9}, dst["b"], "sequences should be replaced, not appended")

	c := dst["c"].(map[string]any)
	assert.Equal(t, 1, c["x"], "untouched map keys preserved")
	assert.Equal(t, 99, c["y"], "overridden")
	assert.Equal(t, 3, c["z"], "new key added")
}
