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
	// noAudit runs Composer with security blocking disabled
	// (COMPOSER_NO_SECURITY_BLOCKING, Composer >= 2.9.2) for the preflight
	// resolution and the upgrade itself. The project's composer.json audit
	// configuration stays untouched.
	noAudit bool

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

// DisableAuditBlock makes the preflight resolution and the upgrade run with
// Composer's security-audit blocking disabled. The user opts into this after
// being warned that dependencies are affected by security advisories.
func (u *ProjectUpgrader) DisableAuditBlock() { u.noAudit = true }

// AuditBlockDisabled reports whether the user chose to continue without
// Composer's security-audit blocking.
func (u *ProjectUpgrader) AuditBlockDisabled() bool { return u.noAudit }

// composerEnv returns the environment for Composer invocations, adding
// COMPOSER_NO_SECURITY_BLOCKING when the user opted out of audit blocking.
// The variable is scoped to the commands the upgrade runs — the project's
// composer.json stays untouched.
func (u *ProjectUpgrader) composerEnv(extra map[string]string) map[string]string {
	env := make(map[string]string, len(extra)+1)
	for k, v := range extra {
		env[k] = v
	}
	if u.noAudit {
		env["COMPOSER_NO_SECURITY_BLOCKING"] = "1"
	}
	return env
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
