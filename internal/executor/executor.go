package executor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var ErrNotSupported = errors.New("operation not supported by this executor")

const (
	TypeDocker     = "docker"
	TypeLocal      = "local"
	TypeSymfonyCLI = "symfony-cli"
)

// StopOptions configures how the development environment is stopped.
type StopOptions struct {
	// RemoveVolumes also removes the named volumes declared in the compose
	// file, deleting all data stored in them.
	RemoveVolumes bool
}

type Executor interface {
	ConsoleCommand(ctx context.Context, args ...string) *Process
	ComposerCommand(ctx context.Context, args ...string) *Process
	PHPCommand(ctx context.Context, args ...string) *Process
	NPMCommand(ctx context.Context, args ...string) *Process
	NormalizePath(hostPath string) string
	Type() string
	WithEnv(env map[string]string) Executor
	WithRelDir(relDir string) Executor
	StartEnvironment(ctx context.Context) error
	StopEnvironment(ctx context.Context, opts StopOptions) error
	EnvironmentStatus(ctx context.Context) (bool, error)
	AdminAPIClient(ctx context.Context) (*adminSdk.Client, error)
	// ShopConfig returns the project config with the selected environment's
	// url and admin_api applied.
	ShopConfig() *shop.Config
	// DatabaseConnection returns credentials to reach the project database
	// from the host machine.
	DatabaseConnection(ctx context.Context) (*DatabaseConnection, error)
}

func adminAPIClient(ctx context.Context, cfg *shop.Config, envCfg *shop.EnvironmentConfig) (*adminSdk.Client, error) {
	if cfg == nil {
		return nil, errors.New("admin api requires a shop configuration")
	}

	effective := *cfg
	if envCfg != nil {
		if envCfg.URL != "" {
			effective.URL = envCfg.URL
		}
		if envCfg.AdminApi != nil {
			effective.AdminApi = envCfg.AdminApi
		}
	}

	return shop.NewShopClient(ctx, &effective)
}

type allowBinCIKey struct{}

type allocateTTYKey struct{}

// WithTTY requests a TTY for docker compose exec (no -T). Use this for
// interactive console usage so Symfony keeps ANSI colors and prompts.
func WithTTY(ctx context.Context) context.Context {
	return context.WithValue(ctx, allocateTTYKey{}, true)
}

func wantsTTY(ctx context.Context) bool {
	v, ok := ctx.Value(allocateTTYKey{}).(bool)
	return ok && v
}

func AllowBinCI(ctx context.Context) context.Context {
	return context.WithValue(ctx, allowBinCIKey{}, true)
}

func IsBinCIAllowed(ctx context.Context) bool {
	_, ok := ctx.Value(allowBinCIKey{}).(bool)
	return ok && isCI()
}

var isCI = sync.OnceValue(func() bool {
	return os.Getenv("CI") != ""
})

func consoleCommandName(ctx context.Context) string {
	if IsBinCIAllowed(ctx) {
		return "bin/ci"
	}
	return "bin/console"
}

func resolveDir(projectRoot, relDir string) string {
	if relDir == "" {
		return projectRoot
	}

	return filepath.Join(projectRoot, relDir)
}

func applyDir(dir string, cmd *exec.Cmd) {
	if dir != "" {
		cmd.Dir = dir
	}
}

func logCmd(ctx context.Context, cmd *exec.Cmd) {
	logging.FromContext(ctx).Debugf("exec: %s (dir: %s)", strings.Join(cmd.Args, " "), cmd.Dir)
}

func mergeEnv(base, extra map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}
