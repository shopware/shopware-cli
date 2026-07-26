package pluginmigrate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
)

// fakeExecutor satisfies executor.Executor and lets each test decide which
// shell command backs composer/console invocations.
type fakeExecutor struct {
	composer func(ctx context.Context, args ...string) *executor.Process
	console  func(ctx context.Context, args ...string) *executor.Process
}

func shellProcess(ctx context.Context, script string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", script)}
}

func (f *fakeExecutor) ComposerCommand(ctx context.Context, args ...string) *executor.Process {
	return f.composer(ctx, args...)
}

func (f *fakeExecutor) ConsoleCommand(ctx context.Context, args ...string) *executor.Process {
	return f.console(ctx, args...)
}

func (f *fakeExecutor) PHPCommand(ctx context.Context, args ...string) *executor.Process {
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) NPMCommand(ctx context.Context, args ...string) *executor.Process {
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) NormalizePath(hostPath string) string            { return hostPath }
func (f *fakeExecutor) Type() string                                    { return executor.TypeLocal }
func (f *fakeExecutor) WithEnv(map[string]string) executor.Executor     { return f }
func (f *fakeExecutor) WithRelDir(string) executor.Executor             { return f }
func (f *fakeExecutor) StartEnvironment(context.Context) error          { return nil }
func (f *fakeExecutor) StopEnvironment(context.Context) error           { return nil }
func (f *fakeExecutor) EnvironmentStatus(context.Context) (bool, error) { return true, nil }
func (f *fakeExecutor) AdminAPIClient(context.Context) (*adminSdk.Client, error) {
	return nil, executor.ErrNotSupported
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// setupProject creates a project with one Store plugin and one local plugin
// in custom/plugins, plus a vendor extension Composer already manages.
func setupProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	writeFile(t, filepath.Join(dir, "composer.json"), `{
		"name": "shopware/production",
		"require": {"shopware/core": "6.6.10.3"}
	}`)
	writeFile(t, filepath.Join(dir, "composer.lock"), `{"packages": [], "packages-dev": []}`)

	writeFile(t, filepath.Join(dir, "vendor", "swag", "demo", "composer.json"), `{
		"name": "swag/demo",
		"type": "shopware-platform-plugin",
		"version": "2.0.0",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-plugin-class": "Swag\\Demo\\Demo", "label": {"en-GB": "Demo"}},
		"autoload": {"psr-4": {"Swag\\Demo\\": "src/"}}
	}`)

	writeFile(t, filepath.Join(dir, "custom", "plugins", "StorePlugin", "composer.json"), `{
		"name": "swag/store-plugin",
		"type": "shopware-platform-plugin",
		"version": "3.1.0",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-plugin-class": "Swag\\StorePlugin\\StorePlugin", "label": {"en-GB": "Store"}},
		"autoload": {"psr-4": {"Swag\\StorePlugin\\": "src/"}}
	}`)

	writeFile(t, filepath.Join(dir, "custom", "plugins", "LocalPlugin", "composer.json"), `{
		"name": "acme/local-plugin",
		"type": "shopware-platform-plugin",
		"version": "1.0.0",
		"require": {"shopware/core": "~6.6.0"},
		"extra": {"shopware-plugin-class": "Acme\\LocalPlugin\\LocalPlugin", "label": {"en-GB": "Local"}},
		"autoload": {"psr-4": {"Acme\\LocalPlugin\\": "src/"}}
	}`)

	return dir
}

func trueExecutor() *fakeExecutor {
	return &fakeExecutor{
		composer: func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "true") },
		console:  func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "true") },
	}
}

func storeSet() map[string]struct{} {
	return map[string]struct{}{"store.shopware.com/storeplugin": {}}
}

func collectEvents(t *testing.T, events <-chan StepEvent) []StepEvent {
	t.Helper()
	var all []StepEvent
	for ev := range events {
		all = append(all, ev)
	}
	return all
}

func finalEvent(t *testing.T, events []StepEvent) StepEvent {
	t.Helper()
	require.NotEmpty(t, events)
	final := events[len(events)-1]
	require.Equal(t, StepFinished, final.Step)
	return final
}

func TestFetchPackageNamesSendsHostKeyedBearerToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// packages.shopware.com rejects unauthenticated requests; the bearer
		// token must be sent on the very first packages.json request.
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"packages": {"store.shopware.com/storeplugin": {"3.1.0": {"name": "store.shopware.com/storeplugin", "version": "3.1.0"}}}}`))
	}))
	defer srv.Close()

	names, err := fetchPackageNames(t.Context(), srv.URL, "valid-token")
	require.NoError(t, err)
	assert.Contains(t, names, "store.shopware.com/storeplugin")

	_, err = fetchPackageNames(t.Context(), srv.URL, "wrong-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "400")
}

func TestScanFindsOnlyCustomExtensions(t *testing.T) {
	dir := setupProject(t)

	scanned := NewPluginMigrator(dir, nil).Scan(t.Context())

	require.Len(t, scanned, 2, "vendor extensions are already Composer-managed")
	assert.Equal(t, "LocalPlugin", scanned[0].Name)
	assert.Equal(t, "acme/local-plugin", scanned[0].ComposerName)
	assert.Equal(t, "custom/plugins/LocalPlugin", scanned[0].RelPath)
	assert.Equal(t, "StorePlugin", scanned[1].Name)
	assert.Equal(t, "3.1.0", scanned[1].Version)
}

func TestBuildPlanClassifiesExtensions(t *testing.T) {
	dir := setupProject(t)
	scanned := NewPluginMigrator(dir, nil).Scan(t.Context())

	plan := BuildPlan(scanned, Availability{Store: storeSet()})

	require.Len(t, plan.Extensions, 2)
	assert.True(t, plan.AddStoreRepository)

	local, store := plan.Extensions[0], plan.Extensions[1]
	assert.Equal(t, ActionPathRepository, local.Kind)
	assert.Equal(t, "acme/local-plugin:*", local.RequireArg)
	assert.Equal(t, ActionStoreRequire, store.Kind)
	assert.Equal(t, "store.shopware.com/storeplugin:3.1.0", store.RequireArg)

	assert.Equal(t, []string{"custom/plugins/LocalPlugin"}, plan.PathRepositories())
	assert.Len(t, plan.RemoveDirs(), 1)
	assert.Contains(t, plan.RemoveDirs()[0], "StorePlugin")
}

func TestBuildPlanWithoutTokenManagesEverythingLocally(t *testing.T) {
	dir := setupProject(t)
	scanned := NewPluginMigrator(dir, nil).Scan(t.Context())

	plan := BuildPlan(scanned, Availability{})

	assert.False(t, plan.AddStoreRepository)
	assert.Equal(t, 2, plan.Count(ActionPathRepository))
	assert.Empty(t, plan.RemoveDirs())
	assert.ElementsMatch(t, []string{"acme/local-plugin:*", "swag/store-plugin:*"}, plan.RequireArgs())
}

func TestBuildPlanUnsupportedWithoutComposerName(t *testing.T) {
	plan := BuildPlan([]ScannedExtension{{Name: "NoComposer"}}, Availability{})

	require.Len(t, plan.Extensions, 1)
	assert.Equal(t, ActionUnsupported, plan.Extensions[0].Kind)
	assert.False(t, plan.Actionable())
}

func TestBuildPlanRequiresPublishedPackages(t *testing.T) {
	dir := setupProject(t)
	scanned := NewPluginMigrator(dir, nil).Scan(t.Context())

	// The local plugin is published on Packagist with the installed version:
	// require it from there (no token involved) and drop the local copy.
	plan := BuildPlan(scanned, Availability{
		Published: map[string][]string{"acme/local-plugin": {"v0.9.0", "v1.0.0"}},
	})

	local := plan.Extensions[0]
	assert.Equal(t, ActionComposerRequire, local.Kind)
	assert.Equal(t, "acme/local-plugin:1.0.0", local.RequireArg)
	assert.False(t, plan.AddStoreRepository)
	assert.NotContains(t, plan.PathRepositories(), "custom/plugins/LocalPlugin")
	require.Len(t, plan.RemoveDirs(), 1)
	assert.Contains(t, plan.RemoveDirs()[0], "LocalPlugin")
}

func TestBuildPlanPublishedWithoutInstalledVersionFallsBack(t *testing.T) {
	dir := setupProject(t)
	scanned := NewPluginMigrator(dir, nil).Scan(t.Context())

	// The repository only offers other releases than the installed 1.0.0 —
	// requiring a different release than the code running in the shop is not
	// a safe migration, so the plugin stays local as a path repository.
	plan := BuildPlan(scanned, Availability{
		Published: map[string][]string{"acme/local-plugin": {"2.0.0"}},
	})

	assert.Equal(t, ActionPathRepository, plan.Extensions[0].Kind)
	assert.Contains(t, plan.PathRepositories(), "custom/plugins/LocalPlugin")
	assert.Empty(t, plan.RemoveDirs())
}

func TestBuildPlanStoreWinsOverPublished(t *testing.T) {
	dir := setupProject(t)
	scanned := NewPluginMigrator(dir, nil).Scan(t.Context())

	plan := BuildPlan(scanned, Availability{
		Store:     storeSet(),
		Published: map[string][]string{"swag/store-plugin": {"3.1.0"}},
	})

	assert.Equal(t, ActionStoreRequire, plan.Extensions[1].Kind)
}

func TestFetchAvailabilityWithoutTokenChecksPublished(t *testing.T) {
	dir := setupProject(t)
	m := NewPluginMigrator(dir, nil)

	var gotNames []string
	m.publishedVersions = func(_ context.Context, names []string) map[string][]string {
		gotNames = names
		return map[string][]string{"acme/local-plugin": {"1.0.0"}}
	}
	m.storePackageNames = func(context.Context, string) (map[string]struct{}, error) {
		t.Fatal("the Store must not be queried without a token")
		return nil, context.Canceled
	}

	avail, err := m.FetchAvailability(t.Context(), "", m.Scan(t.Context()))
	require.NoError(t, err)
	assert.Nil(t, avail.Store)
	assert.ElementsMatch(t, []string{"acme/local-plugin", "swag/store-plugin"}, gotNames)
	assert.Contains(t, avail.Published, "acme/local-plugin")
}

func TestFetchPublishedVersionsUsesConfiguredRepository(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"packages": {"acme/local-plugin": {"1.0.0": {"name": "acme/local-plugin", "version": "1.0.0"}}}}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "composer.json"), `{
		"require": {"shopware/core": "6.6.10.3"},
		"repositories": [{"type": "composer", "url": "`+srv.URL+`"}]
	}`)

	published := NewPluginMigrator(dir, nil).fetchPublishedVersions(t.Context(), []string{"acme/local-plugin"})
	assert.Equal(t, map[string][]string{"acme/local-plugin": {"1.0.0"}}, published)
}

func TestRunMigratesProject(t *testing.T) {
	dir := setupProject(t)
	m := NewPluginMigrator(dir, trueExecutor())
	plan := BuildPlan(m.Scan(t.Context()), Availability{Store: storeSet()})

	events := collectEvents(t, m.Run(t.Context(), plan, "secret-token"))
	require.Equal(t, StateOK, finalEvent(t, events).State)

	composerJSON, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(composerJSON), StoreRepositoryURL)
	assert.Contains(t, string(composerJSON), "custom/plugins/LocalPlugin")

	authJSON, err := os.ReadFile(filepath.Join(dir, "auth.json"))
	require.NoError(t, err)
	assert.Contains(t, string(authJSON), "secret-token")

	_, err = os.Stat(filepath.Join(dir, "custom", "plugins", "StorePlugin"))
	assert.True(t, os.IsNotExist(err), "the Store-migrated directory is removed")
	_, err = os.Stat(filepath.Join(dir, "custom", "plugins", "LocalPlugin"))
	assert.NoError(t, err, "path-repository plugins keep their files")
}

func TestRunFailingRequireRollsBack(t *testing.T) {
	dir := setupProject(t)
	before, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	exec := trueExecutor()
	exec.composer = func(ctx context.Context, _ ...string) *executor.Process {
		return shellProcess(ctx, "echo boom >&2; exit 2")
	}
	m := NewPluginMigrator(dir, exec)
	plan := BuildPlan(m.Scan(t.Context()), Availability{Store: storeSet()})

	events := collectEvents(t, m.Run(t.Context(), plan, "secret-token"))
	final := finalEvent(t, events)
	require.Equal(t, StateFail, final.State)
	assert.Contains(t, final.Err.Error(), "composer require")

	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "composer.json is restored")

	_, err = os.Stat(filepath.Join(dir, "custom", "plugins", "StorePlugin"))
	assert.NoError(t, err, "no directory is removed when the require failed")
}

func TestRunPluginRefreshFailureIsNonFatal(t *testing.T) {
	dir := setupProject(t)
	exec := trueExecutor()
	exec.console = func(ctx context.Context, _ ...string) *executor.Process {
		return shellProcess(ctx, "exit 1")
	}
	m := NewPluginMigrator(dir, exec)
	plan := BuildPlan(m.Scan(t.Context()), Availability{})

	events := collectEvents(t, m.Run(t.Context(), plan, ""))
	assert.Equal(t, StateOK, finalEvent(t, events).State, "plugin:refresh failure does not abort")
}
