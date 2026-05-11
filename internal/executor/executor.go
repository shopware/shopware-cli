package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// ErrNotSupported is returned when the executor does not support a managed environment.
var ErrNotSupported = errors.New("operation not supported by this executor")

// Executor type constants returned by Executor.Type().
const (
	TypeDocker     = "docker"
	TypeLocal      = "local"
	TypeSymfonyCLI = "symfony-cli"
)

// Executor abstracts command execution across different environment types.
type Executor interface {
	ConsoleCommand(ctx context.Context, args ...string) *Process
	ComposerCommand(ctx context.Context, args ...string) *Process
	PHPCommand(ctx context.Context, args ...string) *Process
	NPMCommand(ctx context.Context, args ...string) *Process
	// NormalizePath converts a host-absolute path to the path seen by the execution
	// environment. For local executors the path is returned unchanged; for Docker it
	// is translated to the container mount (e.g. /var/www/html/...).
	NormalizePath(hostPath string) string
	Type() string
	WithEnv(env map[string]string) Executor
	WithRelDir(relDir string) Executor
	// StartEnvironment starts the backing environment (e.g. docker compose up -d).
	// Returns ErrNotSupported for executors that have no managed environment.
	StartEnvironment(ctx context.Context) error
	// StopEnvironment stops the backing environment (e.g. docker compose down).
	// Returns ErrNotSupported for executors that have no managed environment.
	StopEnvironment(ctx context.Context) error
	// AdminAPIClient returns an authenticated admin API client for the configured shop.
	// Credentials come from SHOPWARE_CLI_API_* env vars first, then admin_api in the project config.
	AdminAPIClient(ctx context.Context) (*adminSdk.Client, error)
}

// adminAPIClient is the shared implementation used by all executors. It defers
// to shop.NewShopClient so env vars (SHOPWARE_CLI_API_*) take precedence over
// the project config like elsewhere in the CLI. When an envCfg is supplied
// (the selected named environment from .shopware-project.yml), its URL and
// admin_api block overlay the top-level config so per-environment credentials
// are honored.
func adminAPIClient(ctx context.Context, cfg *shop.Config, envCfg *shop.EnvironmentConfig) (*adminSdk.Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("admin api requires a shop configuration")
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
