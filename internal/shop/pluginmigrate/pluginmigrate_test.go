package pluginmigrate

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
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

func (f *fakeExecutor) NormalizePath(hostPath string) string        { return hostPath }
func (f *fakeExecutor) Type() string                                { return executor.TypeLocal }
func (f *fakeExecutor) WithEnv(map[string]string) executor.Executor { return f }
func (f *fakeExecutor) WithRelDir(string) executor.Executor         { return f }
func (f *fakeExecutor) StartEnvironment(context.Context) error      { return nil }
func (f *fakeExecutor) DatabaseConnection(context.Context) (*executor.DatabaseConnection, error) {
	return nil, executor.ErrNotSupported
}
func (f *fakeExecutor) StopEnvironment(context.Context, executor.StopOptions) error { return nil }
func (f *fakeExecutor) EnvironmentStatus(context.Context) (bool, error)             { return true, nil }
func (f *fakeExecutor) AdminAPIClient(context.Context) (*adminSdk.Client, error) {
	return nil, executor.ErrNotSupported
}
func (f *fakeExecutor) ShopConfig() *shop.Config { return nil }

// setupProject creates a project with one Store plugin and one local plugin
// in custom/plugins, plus a vendor extension Composer already manages.
func setupProject(t *testing.T) string {
	t.Helper()

	p := testhelper.NewProject(t).
		File("composer.json", testhelper.ComposerJSON{
			Name:    "shopware/production",
			Require: map[string]string{"shopware/core": "6.6.10.3"},
		}.String()).
		File("composer.lock", testhelper.ComposerLock()).
		VendorPackage("swag/demo", testhelper.PluginComposer("swag/demo", "2.0.0", `Swag\Demo\Demo`)).
		CustomPlugin("StorePlugin", testhelper.PluginComposer("swag/store-plugin", "3.1.0", `Swag\StorePlugin\StorePlugin`)).
		CustomPlugin("LocalPlugin", testhelper.PluginComposer("acme/local-plugin", "1.0.0", `Acme\LocalPlugin\LocalPlugin`))

	return p.Root
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

func TestScanReturnsAbsolutePathsForRelativeProjectRoot(t *testing.T) {
	dir := setupProject(t)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	relativeRoot, err := filepath.Rel(cwd, dir)
	require.NoError(t, err)

	scanned := NewPluginMigrator(relativeRoot, nil).Scan(t.Context())
	require.NotEmpty(t, scanned)
	for _, extension := range scanned {
		assert.True(t, filepath.IsAbs(extension.Path), extension.Path)
	}
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
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), `{
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
	_, err = os.Stat(m.backupDir())
	assert.True(t, os.IsNotExist(err), "sensitive migration backups are removed after success")
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
	_, err = os.Stat(filepath.Join(dir, "auth.json"))
	assert.True(t, os.IsNotExist(err), "rollback removes auth.json when it did not exist before the run")
	_, err = os.Stat(m.backupDir())
	assert.True(t, os.IsNotExist(err), "sensitive migration backups are removed after failure")
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

func TestBackupRestrictsSensitiveFilePermissions(t *testing.T) {
	dir := setupProject(t)
	testhelper.WriteFile(t, filepath.Join(dir, "auth.json"), `{"bearer": {"packages.shopware.com": "secret"}}`)
	m := NewPluginMigrator(dir, nil)
	t.Cleanup(func() { _ = os.RemoveAll(m.backupDir()) })

	require.NoError(t, m.backup())

	info, err := os.Stat(filepath.Join(m.backupDir(), "auth.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	dirInfo, err := os.Stat(m.backupDir())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestConsumeRunEventsReturnsFailureThatAlsoHasOutput(t *testing.T) {
	runErr := errors.New("migration failed")
	events := make(chan StepEvent, 1)
	events <- StepEvent{Step: StepFinished, State: StateFail, Line: "rollback detail", Err: runErr}
	close(events)

	var out bytes.Buffer
	err := consumeRunEvents(&out, events)

	assert.ErrorIs(t, err, runErr)
	assert.Contains(t, out.String(), "rollback detail")
}

// headlessMigrator returns a migrator with all network seams stubbed.
func headlessMigrator(dir string, exec executor.Executor, store map[string]struct{}, storeErr error) *PluginMigrator {
	m := NewPluginMigrator(dir, exec)
	m.storePackageNames = func(context.Context, string) (map[string]struct{}, error) {
		return store, storeErr
	}
	m.publishedVersions = func(context.Context, []string) map[string][]string {
		return nil
	}
	return m
}

func TestRunHeadlessNothingToDo(t *testing.T) {
	dir := setupProject(t)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "custom")))
	m := headlessMigrator(dir, trueExecutor(), nil, nil)

	var out bytes.Buffer
	require.NoError(t, m.RunHeadless(t.Context(), HeadlessOptions{Out: &out}))
	assert.Contains(t, ansi.Strip(out.String()), "already managed through Composer")
}

func TestRunHeadlessInvalidToken(t *testing.T) {
	dir := setupProject(t)
	m := headlessMigrator(dir, trueExecutor(), nil, errors.New("401 unauthorized"))

	var out bytes.Buffer
	err := m.RunHeadless(t.Context(), HeadlessOptions{Token: "bad", Out: &out})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHOPWARE_PACKAGIST_TOKEN")
}

func TestRunHeadlessDryRun(t *testing.T) {
	dir := setupProject(t)
	before, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	m := headlessMigrator(dir, trueExecutor(), storeSet(), nil)

	var out bytes.Buffer
	require.NoError(t, m.RunHeadless(t.Context(), HeadlessOptions{Token: "tok", DryRun: true, Out: &out}))

	content := ansi.Strip(out.String())
	assert.Contains(t, content, "StorePlugin")
	assert.Contains(t, content, "require from Shopware Store")
	assert.Contains(t, content, "LocalPlugin")
	assert.Contains(t, content, "manage via path repository")
	assert.Contains(t, content, "Dry run — nothing was modified.")

	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestRunHeadlessWithoutTokenExecutes(t *testing.T) {
	dir := setupProject(t)
	m := headlessMigrator(dir, trueExecutor(), nil, nil)

	var out bytes.Buffer
	require.NoError(t, m.RunHeadless(t.Context(), HeadlessOptions{Out: &out}))

	content := ansi.Strip(out.String())
	assert.Contains(t, content, "No SHOPWARE_PACKAGIST_TOKEN set")
	assert.Contains(t, content, "All extensions are now managed through Composer.")

	composerJSON, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(composerJSON), "custom/plugins/LocalPlugin")
	assert.Contains(t, string(composerJSON), "custom/plugins/StorePlugin")
}

func TestRunHeadlessFailingRequireReportsRestore(t *testing.T) {
	dir := setupProject(t)
	exec := trueExecutor()
	exec.composer = func(ctx context.Context, _ ...string) *executor.Process {
		return shellProcess(ctx, "echo boom >&2; exit 2")
	}
	m := headlessMigrator(dir, exec, nil, nil)

	var out bytes.Buffer
	err := m.RunHeadless(t.Context(), HeadlessOptions{Out: &out})
	require.Error(t, err)
	assert.Contains(t, ansi.Strip(out.String()), "were restored")
}

func TestRunHeadlessNothingActionable(t *testing.T) {
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"),
		testhelper.ComposerJSON{Require: map[string]string{"shopware/core": "6.6.10.3"}}.String())
	// An extension without a composer package name cannot be migrated.
	testhelper.WriteFile(t, filepath.Join(dir, "custom", "plugins", "Broken", "composer.json"),
		testhelper.PluginComposer("", "1.0.0", `Broken\Broken`).String())
	m := headlessMigrator(dir, trueExecutor(), nil, nil)

	var out bytes.Buffer
	err := m.RunHeadless(t.Context(), HeadlessOptions{Out: &out})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none of the extensions can be migrated automatically")
}
