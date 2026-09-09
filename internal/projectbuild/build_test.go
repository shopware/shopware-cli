package projectbuild

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

// Keep the real local executor's environment handling, but replace PHP and
// Composer with a shell process so pipeline tests never install dependencies.
type buildTestExecutor struct {
	executor.Executor
	composerArgs *[]string
	composerErr  error
}

func (e *buildTestExecutor) WithEnv(env map[string]string) executor.Executor {
	clone := *e
	clone.Executor = e.Executor.WithEnv(env)
	return &clone
}

func (e *buildTestExecutor) command(ctx context.Context, label string) *executor.Process {
	p := e.NPMCommand(ctx)
	cmd := exec.CommandContext(ctx, "sh", "-c", `
test "$APP_ENV" = prod &&
test "$COMPOSER_ROOT_VERSION" = 1.0.0 &&
test "$SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION" = 1 &&
printf '%s\n' "$1" >> steps
`, "build-test", label)
	cmd.Env = p.Cmd.Env
	cmd.Dir = p.Cmd.Dir
	return &executor.Process{Cmd: cmd}
}

func (e *buildTestExecutor) ComposerCommand(ctx context.Context, args ...string) *executor.Process {
	*e.composerArgs = args
	p := e.command(ctx, "composer")
	p.Cmd.Err = e.composerErr
	return p
}

func (e *buildTestExecutor) PHPCommand(ctx context.Context, args ...string) *executor.Process {
	return e.command(ctx, strings.Join(args, " "))
}

func TestBuildPipeline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("hooks run through sh -c")
	}
	t.Setenv("APP_ENV", "")
	t.Setenv("COMPOSER_ROOT_VERSION", "")
	t.Setenv("SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION", "")
	t.Setenv("COMPOSER_AUTH", "")
	t.Setenv("SHOPWARE_PACKAGES_TOKEN", "")

	for _, withDev := range []bool{false, true} {
		t.Run(map[bool]string{false: "production dependencies", true: "dev dependencies"}[withDev], func(t *testing.T) {
			root := t.TempDir()
			testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
			testhelper.WriteFile(t, filepath.Join(root, "remove-me"), "source")
			testhelper.WriteFile(t, filepath.Join(root, "var/cache/stale"), "cache")
			cfg := &shop.Config{
				DisableComposerScripts: true,
				Build: &shop.ConfigBuild{
					CleanupPaths:     []string{"remove-me"},
					DisableChecksums: true,
					Hooks: &shop.ConfigBuildHooks{
						Pre:          []string{"echo pre >> steps"},
						PreComposer:  []string{"echo pre-composer >> steps"},
						PostComposer: []string{"echo post-composer >> steps"},
						PreAssets:    []string{"echo pre-assets >> steps"},
						PostAssets:   []string{"echo post-assets >> steps"},
						Post:         []string{"echo post >> steps"},
					},
				},
			}
			var composerArgs []string
			e := &buildTestExecutor{Executor: executor.NewLocal(root), composerArgs: &composerArgs}
			require.NoError(t, run(t.Context(), root, cfg, e, Options{WithDevDependencies: withDev}))

			steps, err := os.ReadFile(filepath.Join(root, "steps"))
			require.NoError(t, err)
			assert.Equal(t, "pre\npre-composer\ncomposer\npost-composer\npre-assets\npost-assets\nbin/ci --version\nbin/ci asset:install\npost\n", string(steps))
			assert.Equal(t, !withDev, strings.Contains(strings.Join(composerArgs, " "), "--no-dev"))
			assert.Contains(t, composerArgs, "--no-scripts")
			assert.NoFileExists(t, filepath.Join(root, "remove-me"))
			assert.NoDirExists(t, filepath.Join(root, "var/cache"))
			assert.FileExists(t, filepath.Join(root, "vendor/shopware/administration/Resources/app/administration/src/app/snippet/.gitkeep"))
			assert.Equal(t, []string{"remove-me"}, cfg.Build.CleanupPaths)
			assert.Empty(t, os.Getenv("APP_ENV"))
			assert.Empty(t, os.Getenv("COMPOSER_ROOT_VERSION"))
			assert.Empty(t, os.Getenv("SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION"))
		})
	}
}

func TestBuildStopsOnComposerFailure(t *testing.T) {
	root := t.TempDir()
	var args []string
	buildErr := errors.New("composer failed")
	e := &buildTestExecutor{Executor: executor.NewLocal(root), composerArgs: &args, composerErr: buildErr}
	t.Setenv("COMPOSER_AUTH", "")
	t.Setenv("SHOPWARE_PACKAGES_TOKEN", "")
	testhelper.WriteFile(t, filepath.Join(root, "remove-me"), "source")
	cfg := &shop.Config{Build: &shop.ConfigBuild{CleanupPaths: []string{"remove-me"}}}
	require.ErrorIs(t, run(t.Context(), root, cfg, e, Options{}), buildErr)
	assert.FileExists(t, filepath.Join(root, "remove-me"))
	assert.NoDirExists(t, filepath.Join(root, "vendor"))
}
