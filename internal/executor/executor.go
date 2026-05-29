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

var ErrNotSupported = errors.New("operation not supported by this executor")

const (
	TypeDocker     = "docker"
	TypeLocal      = "local"
	TypeSymfonyCLI = "symfony-cli"
)

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
	StopEnvironment(ctx context.Context) error
	EnvironmentStatus(ctx context.Context) (bool, error)
	AdminAPIClient(ctx context.Context) (*adminSdk.Client, error)
}

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
