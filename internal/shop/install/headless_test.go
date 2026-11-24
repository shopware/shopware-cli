package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

func notInstalledConsole(ctx context.Context, _ ...string) *executor.Process {
	return shellProcess(ctx, "false")
}

func TestRunHeadlessSkipsWhenInstalled(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")

	fake := &fakeExecutor{
		php: func(ctx context.Context, _ ...string) *executor.Process {
			t.Fatal("the deployment helper must not run when Shopware is already installed")
			return nil
		},
	}

	var out bytes.Buffer
	cfg := &shop.Config{}
	envCfg := &shop.EnvironmentConfig{}
	err := RunHeadless(t.Context(), fake, cfg, envCfg, t.TempDir(), HeadlessOptions{Out: &out})

	require.NoError(t, err)
	plain := ansi.Strip(out.String())
	assert.Contains(t, plain, "already installed")
	assert.Contains(t, plain, "project dev stop --remove-data")
}

func TestRunHeadlessInstallsAndPersists(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := t.TempDir()

	fake := &fakeExecutor{
		console: notInstalledConsole,
		php: func(ctx context.Context, _ ...string) *executor.Process {
			return shellProcess(ctx, `echo "Start: bin/console system:install"; echo migrated; echo "Start: bin/console user:create admin"`)
		},
	}

	envCfg := &shop.EnvironmentConfig{Type: "docker", URL: "http://localhost:8000"}
	cfg := &shop.Config{Environments: map[string]*shop.EnvironmentConfig{"local": envCfg}}

	var out bytes.Buffer
	err := RunHeadless(t.Context(), fake, cfg, envCfg, dir, HeadlessOptions{
		Install: Options{AdminPassword: "secret123"},
		Out:     &out,
	})
	require.NoError(t, err)

	assert.Equal(t, "en-GB", fake.env["INSTALL_LOCALE"], "defaults are applied to empty fields")
	assert.Equal(t, "secret123", fake.env["INSTALL_ADMIN_PASSWORD"])
	assert.NotContains(t, fake.env, ForceReinstallEnv)
	assert.NoFileExists(t, pendingMarkerPath(dir))

	plain := ansi.Strip(out.String())
	assert.Contains(t, plain, "▸ Installing Shopware")
	assert.NotContains(t, plain, ResumeNotice, "a fresh install prints no resume notice")
	assert.Contains(t, plain, "▸ Creating admin account")
	assert.Contains(t, plain, "migrated", "raw helper output is echoed")
	assert.Contains(t, plain, "Shopware installed in")
	assert.Contains(t, plain, "http://localhost:8000/admin")
	assert.Contains(t, plain, "Admin user: admin")

	written, err := os.ReadFile(filepath.Join(dir, ".config/shopware-project.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "password: secret123")
}

func TestRunHeadlessResumesUnfinishedInstall(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := t.TempDir()

	_, err := MarkPending(dir)
	require.NoError(t, err)

	helperRan := false
	fake := &fakeExecutor{
		php: func(ctx context.Context, _ ...string) *executor.Process {
			helperRan = true
			return shellProcess(ctx, "true")
		},
	}

	var out bytes.Buffer
	err = RunHeadless(t.Context(), fake, &shop.Config{}, &shop.EnvironmentConfig{}, dir, HeadlessOptions{Out: &out})
	require.NoError(t, err)

	assert.Contains(t, ansi.Strip(out.String()), ResumeNotice)
	assert.True(t, helperRan, "an unfinished install is installed again although system:is-installed succeeds")
	assert.Equal(t, "1", fake.env[ForceReinstallEnv])
	assert.NoFileExists(t, pendingMarkerPath(dir))
}

func TestRunHeadlessFailureNamesStep(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := t.TempDir()

	fake := &fakeExecutor{
		console: notInstalledConsole,
		php: func(ctx context.Context, _ ...string) *executor.Process {
			return shellProcess(ctx, `echo "Start: bin/console system:install"; echo "Start: bin/console user:create admin"; exit 1`)
		},
	}

	var out bytes.Buffer
	cfg := &shop.Config{}
	envCfg := &shop.EnvironmentConfig{}
	err := RunHeadless(t.Context(), fake, cfg, envCfg, dir, HeadlessOptions{Out: &out})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "installing Shopware")
	assert.Contains(t, ansi.Strip(out.String()), "failed during user:create")
	assert.FileExists(t, pendingMarkerPath(dir), "a failed install keeps the marker")
}

func TestRunHeadlessValidatesOptions(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")

	fake := &fakeExecutor{
		console: func(ctx context.Context, _ ...string) *executor.Process {
			t.Fatal("invalid options must fail before any command runs")
			return nil
		},
	}

	err := RunHeadless(t.Context(), fake, &shop.Config{}, &shop.EnvironmentConfig{}, t.TempDir(), HeadlessOptions{
		Install: Options{Locale: "xx-XX"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown locale")
}

func TestRunHeadlessStopsWhenMarkerCannotBeWritten(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".shopware-cli"), nil, 0o644))

	helperRan := false
	fake := &fakeExecutor{
		console: notInstalledConsole,
		php: func(ctx context.Context, _ ...string) *executor.Process {
			helperRan = true
			return shellProcess(ctx, "true")
		},
	}
	err := RunHeadless(t.Context(), fake, &shop.Config{}, &shop.EnvironmentConfig{}, dir, HeadlessOptions{})
	require.Error(t, err)
	assert.False(t, helperRan)
}

func TestRunHeadlessWarnsWhenMarkerCannotBeRemoved(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	t.Setenv("DO_NOT_TRACK", "1")
	dir := t.TempDir()
	markerDir := filepath.Join(dir, ".shopware-cli", "install")
	t.Cleanup(func() { _ = os.Chmod(markerDir, 0o755) })

	fake := &fakeExecutor{
		console: notInstalledConsole,
		php: func(ctx context.Context, _ ...string) *executor.Process {
			return shellProcess(ctx, "chmod 555 '"+markerDir+"'")
		},
	}

	core, logs := observer.New(zap.WarnLevel)
	ctx := logging.WithLogger(t.Context(), zap.New(core).Sugar())

	err := RunHeadless(ctx, fake, &shop.Config{}, &shop.EnvironmentConfig{}, dir, HeadlessOptions{Out: &bytes.Buffer{}})
	require.NoError(t, err, "a marker that stays does not fail the installation")

	require.Equal(t, 1, logs.Len())
	assert.Contains(t, logs.All()[0].Message, pendingMarkerPath(dir))
}
