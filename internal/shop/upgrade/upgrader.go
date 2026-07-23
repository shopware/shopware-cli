package upgrade

import (
	"context"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
)

// ProjectUpgrader is the entry point for upgrading a Shopware project: it
// runs the readiness checks, loads the version catalog, checks extension
// compatibility, verifies Composer can resolve the upgrade, and executes it.
//
// External dependencies live in unexported fields with production defaults so
// tests can replace them per instance.
type ProjectUpgrader struct {
	projectRoot string
	executor    executor.Executor

	shopwareVersions func(ctx context.Context) ([]string, error)
	extensionUpdates func(ctx context.Context, current, future string, extensions []account_api.UpdateCheckExtension) ([]account_api.UpdateCheckExtensionCompatibility, error)
	repositories     func(*composer.Json, *composer.Auth) *repository.Set
	endOfLifeURL     string
	packagistPingURL string
}

// NewProjectUpgrader creates an upgrader for the project at projectRoot. The
// executor runs composer and the deployment helper in the project's
// environment during Run; the earlier, read-only phases don't use it.
func NewProjectUpgrader(projectRoot string, exec executor.Executor) *ProjectUpgrader {
	return &ProjectUpgrader{
		projectRoot: projectRoot,
		executor:    exec,

		shopwareVersions: extension.GetShopwareVersions,
		extensionUpdates: account_api.GetFutureExtensionUpdates,
		repositories: func(c *composer.Json, auth *composer.Auth) *repository.Set {
			return repository.FromComposer(c, auth, true)
		},
		endOfLifeURL:     "https://endoflife.date/api/shopware.json",
		packagistPingURL: "https://repo.packagist.org/packages.json",
	}
}

// InstalledPHPVersion returns the PHP version of the environment the upgrade
// runs in, or "" when it cannot be determined.
func (u *ProjectUpgrader) InstalledPHPVersion(ctx context.Context) string {
	out, err := u.executor.PHPCommand(ctx, "-r", "echo PHP_VERSION;").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
