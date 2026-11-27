package install

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

// fakeExecutor satisfies executor.Executor and lets each test decide which
// shell command backs console/php invocations.
type fakeExecutor struct {
	console func(ctx context.Context, args ...string) *executor.Process
	php     func(ctx context.Context, args ...string) *executor.Process
	// env records the last WithEnv call so tests can assert the variables
	// the deployment helper runs with.
	env map[string]string
	// executorType is returned by Type; empty means local.
	executorType string
}

func shellProcess(ctx context.Context, script string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", script)}
}

func (f *fakeExecutor) ConsoleCommand(ctx context.Context, args ...string) *executor.Process {
	if f.console != nil {
		return f.console(ctx, args...)
	}
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) PHPCommand(ctx context.Context, args ...string) *executor.Process {
	if f.php != nil {
		return f.php(ctx, args...)
	}
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) ComposerCommand(ctx context.Context, args ...string) *executor.Process {
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) NPMCommand(ctx context.Context, args ...string) *executor.Process {
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) AvailableLogFiles(context.Context) ([]executor.LogFile, error) {
	return nil, executor.ErrNotSupported
}

func (f *fakeExecutor) GetLog(context.Context, string, int, bool, io.Writer) error {
	return nil
}

func (f *fakeExecutor) NormalizePath(hostPath string) string { return hostPath }
func (f *fakeExecutor) Type() string {
	if f.executorType != "" {
		return f.executorType
	}
	return executor.TypeLocal
}
func (f *fakeExecutor) WithEnv(env map[string]string) executor.Executor {
	f.env = env
	return f
}
func (f *fakeExecutor) WithRelDir(string) executor.Executor    { return f }
func (f *fakeExecutor) StartEnvironment(context.Context) error { return nil }
func (f *fakeExecutor) StopEnvironment(context.Context, executor.StopOptions) error {
	return nil
}
func (f *fakeExecutor) EnvironmentStatus(context.Context) (bool, error) { return true, nil }
func (f *fakeExecutor) AdminAPIClient(context.Context) (*adminSdk.Client, error) {
	return nil, executor.ErrNotSupported
}
func (f *fakeExecutor) ShopConfig() *shop.Config { return nil }
func (f *fakeExecutor) DatabaseConnection(context.Context, ...executor.DatabaseEndpoint) (*executor.DatabaseConnection, error) {
	return nil, executor.ErrNotSupported
}

func TestApplyDefaults(t *testing.T) {
	var opts Options
	opts.ApplyDefaults()
	assert.Equal(t, Options{
		Locale:        "en-GB",
		Currency:      "EUR",
		AdminUsername: "admin",
		AdminPassword: "shopware",
	}, opts)

	opts = Options{Locale: "de-DE", Currency: "CHF", AdminUsername: "boss", AdminPassword: "secret123"}
	opts.ApplyDefaults()
	assert.Equal(t, "de-DE", opts.Locale)
	assert.Equal(t, "CHF", opts.Currency)
	assert.Equal(t, "boss", opts.AdminUsername)
	assert.Equal(t, "secret123", opts.AdminPassword)
}

func TestValidate(t *testing.T) {
	valid := Options{Locale: "de-DE", Currency: "EUR", AdminUsername: "admin", AdminPassword: "shopware"}

	cases := []struct {
		name    string
		mutate  func(*Options)
		wantErr string
	}{
		{"valid", func(*Options) {}, ""},
		{"unknown locale", func(o *Options) { o.Locale = "xx-XX" }, "unknown locale"},
		{"unknown currency", func(o *Options) { o.Currency = "BTC" }, "unknown currency"},
		{"short password", func(o *Options) { o.AdminPassword = "short" }, "at least 8 characters"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := valid
			tc.mutate(&opts)
			err := opts.Validate()
			if tc.wantErr == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestValidateAdminPasswordCountsRunes(t *testing.T) {
	assert.NoError(t, ValidateAdminPassword("äöüäöüäö"), "8 multibyte runes are enough")
	assert.Error(t, ValidateAdminPassword("äöüäöüä"))
}

func TestValidateErrorListsValidValues(t *testing.T) {
	err := ValidateLocale("klingon")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "en-GB")
	assert.Contains(t, err.Error(), "sv-SE")

	err = ValidateCurrency("gold")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EUR")
	assert.Contains(t, err.Error(), "CZK")
}

func TestCustomCredentials(t *testing.T) {
	opts := Options{AdminUsername: "admin", AdminPassword: "shopware"}
	assert.False(t, opts.CustomCredentials())

	opts.AdminPassword = "different1"
	assert.True(t, opts.CustomCredentials())

	opts = Options{AdminUsername: "boss", AdminPassword: "shopware"}
	assert.True(t, opts.CustomCredentials())
}

func TestLocaleIDs(t *testing.T) {
	ids := LocaleIDs()
	assert.Len(t, ids, len(Languages))
	assert.Equal(t, "en-GB", ids[0])
}

func TestMatchStep(t *testing.T) {
	idx, ok := MatchStep("Start: bin/console system:install --create-database", 0)
	require.True(t, ok)
	assert.Equal(t, 0, idx)

	idx, ok = MatchStep("Start: bin/console user:create admin", 0)
	require.True(t, ok)
	assert.Equal(t, 1, idx)

	_, ok = MatchStep("running system:install now", 0)
	assert.False(t, ok, "lines without the Start: prefix never match")

	_, ok = MatchStep("Start: bin/console system:install", 1)
	assert.False(t, ok, "progress is monotonic — earlier steps do not match again")

	idx, ok = MatchStep("Start: bin/console theme:change --all", 1)
	require.True(t, ok)
	assert.Equal(t, 4, idx, "steps may be skipped")
}

func TestFailedStep(t *testing.T) {
	assert.Equal(t, "system:install", FailedStep(0))
	assert.Equal(t, "user:create", FailedStep(1))
	assert.Equal(t, "plugin:refresh", FailedStep(len(Steps)+5), "out-of-range clamps to the last step")
}

func TestIsInstalled(t *testing.T) {
	dir := t.TempDir()

	installed := &fakeExecutor{}
	assert.True(t, IsInstalled(t.Context(), installed, dir))

	notInstalled := &fakeExecutor{
		console: func(ctx context.Context, args ...string) *executor.Process {
			assert.Equal(t, []string{"system:is-installed"}, args)
			return shellProcess(ctx, "false")
		},
	}
	assert.False(t, IsInstalled(t.Context(), notInstalled, dir))

	_, err := MarkPending(dir)
	require.NoError(t, err)
	assert.False(t, IsInstalled(t.Context(), installed, dir), "an unfinished installation counts as not installed")
}

func TestMarkPending(t *testing.T) {
	dir := t.TempDir()

	resume, err := MarkPending(dir)
	require.NoError(t, err)
	assert.False(t, resume)
	assert.FileExists(t, pendingMarkerPath(dir))

	gitignore := filepath.Join(dir, ".shopware-cli", "install", ".gitignore")
	content, err := os.ReadFile(gitignore)
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(content))

	require.NoError(t, os.WriteFile(gitignore, []byte("custom\n"), 0o644))

	resume, err = MarkPending(dir)
	require.NoError(t, err)
	assert.True(t, resume)

	content, err = os.ReadFile(gitignore)
	require.NoError(t, err)
	assert.Equal(t, "custom\n", string(content), "an existing .gitignore is kept")
}

func TestIsPending(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, IsPending(dir))

	_, err := MarkPending(dir)
	require.NoError(t, err)
	assert.True(t, IsPending(dir))

	require.NoError(t, ClearPending(dir))
	assert.False(t, IsPending(dir))
}

func TestMarkPendingFails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".shopware-cli"), nil, 0o644))

	_, err := MarkPending(dir)
	require.Error(t, err)
}

func TestClearPending(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, ClearPending(dir), "a missing marker is not an error")

	_, err := MarkPending(dir)
	require.NoError(t, err)
	require.NoError(t, ClearPending(dir))
	assert.NoFileExists(t, pendingMarkerPath(dir))
}

func TestRunPassesEnvAndStreamsLines(t *testing.T) {
	fake := &fakeExecutor{
		php: func(ctx context.Context, args ...string) *executor.Process {
			assert.Equal(t, []string{"vendor/bin/shopware-deployment-helper", "run"}, args)
			return shellProcess(ctx, "echo one; echo two 1>&2")
		},
	}

	var lines []string
	err := Run(t.Context(), fake, Options{
		Locale:        "de-DE",
		Currency:      "CHF",
		AdminUsername: "boss",
		AdminPassword: "secret123",
	}, func(line string) { lines = append(lines, line) })

	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"INSTALL_LOCALE":         "de-DE",
		"INSTALL_CURRENCY":       "CHF",
		"INSTALL_ADMIN_USERNAME": "boss",
		"INSTALL_ADMIN_PASSWORD": "secret123",
	}, fake.env, "no force reinstall without ForceReinstall")
	assert.ElementsMatch(t, []string{"one", "two"}, lines, "stdout and stderr are both streamed")

	err = Run(t.Context(), fake, Options{ForceReinstall: true}, func(string) {})
	require.NoError(t, err)
	assert.Equal(t, "1", fake.env[ForceReinstallEnv])
}

func TestRunReturnsHelperError(t *testing.T) {
	fake := &fakeExecutor{
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo boom; exit 3")
		},
	}

	var lines []string
	err := Run(t.Context(), fake, Options{}, func(line string) { lines = append(lines, line) })
	require.Error(t, err)
	assert.Equal(t, []string{"boom"}, lines)
}

func TestPersistCredentialsStoredEnvironment(t *testing.T) {
	dir := t.TempDir()
	envCfg := &shop.EnvironmentConfig{Type: "docker", URL: "http://localhost:8000"}
	cfg := &shop.Config{Environments: map[string]*shop.EnvironmentConfig{"local": envCfg}}

	opts := Options{AdminUsername: "boss", AdminPassword: "secret123"}
	require.NoError(t, PersistCredentials(cfg, envCfg, dir, opts))

	assert.Nil(t, cfg.AdminApi, "a stored environment does not touch the deprecated top-level admin_api")

	written, err := os.ReadFile(filepath.Join(dir, ".config/shopware-project.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "username: boss")
	assert.Contains(t, string(written), "password: secret123")
}

func TestPersistCredentialsTopLevelCopy(t *testing.T) {
	dir := t.TempDir()

	// A config in the deprecated top-level form: ResolveEnvironment returns a
	// copy that is not stored in cfg.Environments.
	cfg := &shop.Config{URL: "http://localhost:8000"}
	envCfg, err := cfg.ResolveEnvironment("")
	require.NoError(t, err)

	opts := Options{AdminUsername: "boss", AdminPassword: "secret123"}
	require.NoError(t, PersistCredentials(cfg, envCfg, dir, opts))

	require.NotNil(t, cfg.AdminApi, "credentials must land on the config that gets written, not only the copy")
	assert.Equal(t, "boss", cfg.AdminApi.Username)

	written, err := os.ReadFile(filepath.Join(dir, ".config/shopware-project.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "username: boss")
}

func TestMarkPendingIsInvisibleToGit(t *testing.T) {
	testhelper.RequireTool(t, "git")
	dir := t.TempDir()
	require.NoError(t, exec.CommandContext(t.Context(), "git", "-C", dir, "init", "-q").Run())

	_, err := MarkPending(dir)
	require.NoError(t, err)
	assert.FileExists(t, pendingMarkerPath(dir))

	out, err := exec.CommandContext(t.Context(), "git", "-C", dir, "status", "--porcelain", "--untracked-files=all").Output()
	require.NoError(t, err)
	assert.Empty(t, strings.TrimSpace(string(out)))
}

func TestClearPendingWarningNamesAbsoluteMarkerPath(t *testing.T) {
	t.Chdir(t.TempDir())

	warning := ClearPendingWarning("project", errors.New("permission denied"))

	abs, err := filepath.Abs(filepath.Join("project", ".shopware-cli", "install", "pending"))
	require.NoError(t, err)
	assert.Contains(t, warning, abs)
}

func TestIsInstalledCreatesInstallLockForDocker(t *testing.T) {
	notInstalled := func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "false") }

	t.Run("creates install.lock for an installed Docker shop", func(t *testing.T) {
		root := t.TempDir()

		assert.True(t, IsInstalled(t.Context(), &fakeExecutor{executorType: executor.TypeDocker}, root))
		assert.FileExists(t, filepath.Join(root, "install.lock"))
	})

	t.Run("keeps an existing install.lock", func(t *testing.T) {
		root := t.TempDir()
		lock := filepath.Join(root, "install.lock")
		require.NoError(t, os.WriteFile(lock, []byte("2026-11-25"), 0o644))

		assert.True(t, IsInstalled(t.Context(), &fakeExecutor{executorType: executor.TypeDocker}, root))
		content, err := os.ReadFile(lock)
		require.NoError(t, err)
		assert.Equal(t, "2026-11-25", string(content))
	})

	t.Run("leaves a shop that is not installed alone", func(t *testing.T) {
		root := t.TempDir()

		assert.False(t, IsInstalled(t.Context(), &fakeExecutor{executorType: executor.TypeDocker, console: notInstalled}, root))
		assert.NoFileExists(t, filepath.Join(root, "install.lock"))
	})

	t.Run("leaves other executors alone", func(t *testing.T) {
		root := t.TempDir()

		assert.True(t, IsInstalled(t.Context(), &fakeExecutor{}, root))
		assert.NoFileExists(t, filepath.Join(root, "install.lock"))
	})
}
