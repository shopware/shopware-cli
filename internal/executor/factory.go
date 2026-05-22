package executor

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/shopware/shopware-cli/internal/shop"
)

func New(projectRoot string, cfg *shop.EnvironmentConfig, shopCfg *shop.Config) (Executor, error) {
	switch cfg.Type {
	case TypeLocal, "":
		if shopCfg.IsCompatibilityDateBefore(shop.CompatibilityDevMode) {
			if path := pathToSymfonyCLI(); path != "" && symfonyCliAllowed() {
				return &SymfonyCLIExecutor{BinaryPath: path, projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
			}
		}
		return NewLocalWithConfig(projectRoot, cfg, shopCfg), nil
	case TypeSymfonyCLI:
		path := pathToSymfonyCLI()
		if path == "" {
			return nil, fmt.Errorf("symfony CLI not found in PATH")
		}
		return &SymfonyCLIExecutor{BinaryPath: path, projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	case TypeDocker:
		return &DockerExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unsupported environment type: %s", cfg.Type)
	}
}

// NewLocal returns a local executor for the given project root.
func NewLocal(projectRoot string) Executor {
	return NewLocalWithConfig(projectRoot, nil, nil)
}

// NewLocalWithConfig returns a local executor for the given project root and project configuration.
func NewLocalWithConfig(projectRoot string, cfg *shop.EnvironmentConfig, shopCfg *shop.Config) Executor {
	return &LocalExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}
}

var pathToSymfonyCLI = sync.OnceValue(func() string {
	path, err := exec.LookPath("symfony")
	if err != nil {
		return ""
	}
	return path
})

func symfonyCliAllowed() bool {
	return os.Getenv("SHOPWARE_CLI_NO_SYMFONY_CLI") != "1"
}
