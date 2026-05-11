package executor

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/shopware/shopware-cli/internal/shop"
)

// New creates an Executor for the given environment, shop configuration, and project root directory.
func New(projectRoot string, cfg *shop.EnvironmentConfig, shopCfg *shop.Config) (Executor, error) {
	switch cfg.Type {
	case "local", "":
		if shopCfg.IsCompatibilityDateBefore(shop.CompatibilityDevMode) {
			if path := pathToSymfonyCLI(); path != "" && symfonyCliAllowed() {
				return &SymfonyCLIExecutor{BinaryPath: path, projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
			}
		}
		return &LocalExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	case "symfony-cli":
		path := pathToSymfonyCLI()
		if path == "" {
			return nil, fmt.Errorf("symfony CLI not found in PATH")
		}
		return &SymfonyCLIExecutor{BinaryPath: path, projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	case "docker":
		return &DockerExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unsupported environment type: %s", cfg.Type)
	}
}

// NewLocal creates a LocalExecutor with the given project root directory.
// AdminAPIClient on the returned executor will error because no shop config is bound;
// use New for that.
func NewLocal(projectRoot string) Executor {
	return &LocalExecutor{projectRoot: projectRoot}
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
