package executor

import (
	"errors"
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
	case TypeSSH:
		if cfg.SSH == nil {
			return nil, errors.New("ssh environment requires an ssh section with host and directory")
		}
		if cfg.SSH.Host == "" {
			return nil, errors.New("ssh environment requires ssh.host")
		}
		if cfg.SSH.Directory == "" {
			return nil, errors.New("ssh environment requires ssh.directory")
		}
		return &SSHExecutor{host: cfg.SSH.Host, user: cfg.SSH.User, port: cfg.SSH.Port, directory: cfg.SSH.Directory, identityFile: cfg.SSH.IdentityFile, projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	case TypeSymfonyCLI:
		path := pathToSymfonyCLI()
		if path == "" {
			return nil, errors.New("symfony CLI not found in PATH")
		}
		return &SymfonyCLIExecutor{BinaryPath: path, projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg}, nil
	case TypeDocker:
		// Snapshot the Compose project name from .env now — Compose re-reads
		// .env per command, so a later rewrite of that file must not detach
		// the executor from the containers it started with. A process-level
		// COMPOSE_PROJECT_NAME outranks .env in Compose's own precedence and
		// is inherited by every docker invocation, so it stays authoritative.
		composeProjectName := ""
		if os.Getenv(shop.ComposeProjectNameEnvKey) == "" {
			composeProjectName = shop.ReadComposeProjectName(projectRoot)
		}
		return &DockerExecutor{projectRoot: projectRoot, shopCfg: shopCfg, envCfg: cfg, composeProjectName: composeProjectName}, nil
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
