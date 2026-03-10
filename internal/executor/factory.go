package executor

import (
	"fmt"
	"os"
	"os/exec"
	"sync"

	"github.com/shopware/shopware-cli/internal/shop"
)

// New creates an Executor for the given environment and shop configuration.
func New(cfg *shop.EnvironmentConfig, shopCfg *shop.Config) (Executor, error) {
	switch cfg.Type {
	case "local", "":
		if shopCfg.IsCompatibilityDateBefore(shop.CompatibilityDevMode) {
			if path := pathToSymfonyCLI(); path != "" && symfonyCliAllowed() {
				return &SymfonyCLIExecutor{BinaryPath: path}, nil
			}
		}
		return &LocalExecutor{}, nil
	case "symfony-cli":
		path := pathToSymfonyCLI()
		if path == "" {
			return nil, fmt.Errorf("symfony CLI not found in PATH")
		}
		return &SymfonyCLIExecutor{BinaryPath: path}, nil
	case "docker":
		return &DockerExecutor{}, nil
	default:
		return nil, fmt.Errorf("unsupported environment type: %s", cfg.Type)
	}
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
