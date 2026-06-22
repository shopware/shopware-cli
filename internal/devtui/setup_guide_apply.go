package devtui

import (
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
)

func (sg *setupGuide) applyToConfig(cfg *shop.Config) {
	c := sg.currentConfig()

	// Always update compatibility_date to support dev mode
	cfg.CompatibilityDate = shop.CompatibilityDevMode

	// Set URL at top level for backwards compatibility
	if cfg.URL == "" {
		cfg.URL = c.url
	}

	// Set up local environment as Docker
	envCfg := &shop.EnvironmentConfig{
		Type: "docker",
		URL:  c.url,
	}
	if c.username != "" || c.password != "" {
		envCfg.AdminApi = &shop.ConfigAdminApi{
			Username: c.username,
			Password: c.password,
		}
	}
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]*shop.EnvironmentConfig)
	}
	cfg.Environments["local"] = envCfg

	// Set Docker config
	if cfg.Docker == nil {
		cfg.Docker = &shop.ConfigDocker{}
	}
	if cfg.Docker.PHP == nil {
		cfg.Docker.PHP = &shop.ConfigDockerPHP{}
	}
	cfg.Docker.PHP.Version = c.phpVersion
}

// ensureDeploymentHelper adds shopware/deployment-helper to the project's
// composer.json require block when it's missing. New projects created via
// `shopware-cli project create` pin this package; older projects being
// migrated to dev mode need it added so devtui can run
// `vendor/bin/shopware-deployment-helper`.
//
// Returns true when composer.json was changed and the user should re-run
// `composer install` (or `composer update`) to pull the package in.
// Errors reading or writing composer.json are returned to the caller;
// a missing composer.json is treated as nothing-to-do (returns false, nil).
func ensureDeploymentHelper(projectRoot string) (changed bool, err error) {
	composerPath := filepath.Join(projectRoot, "composer.json")
	if _, statErr := os.Stat(composerPath); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return false, statErr
	}

	cj, err := packagist.ReadComposerJson(composerPath)
	if err != nil {
		return false, err
	}

	if cj.HasPackage("shopware/deployment-helper") || cj.HasPackageDev("shopware/deployment-helper") {
		return false, nil
	}

	if cj.Require == nil {
		cj.Require = packagist.ComposerPackageLink{}
	}
	cj.Require["shopware/deployment-helper"] = "*"

	if err := cj.Save(); err != nil {
		return false, err
	}
	return true, nil
}
