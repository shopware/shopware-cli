package upgrade

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/logging"
)

const deploymentHelperPackage = "shopware/deployment-helper"

// RunReadinessChecks inspects the project and returns the readiness checklist
// for the wizard's first step. It is read-only and can be re-run ("Recheck")
// after the user fixes a failing check.
func (u *ProjectUpgrader) RunReadinessChecks(ctx context.Context) Readiness {
	var r Readiness

	r.Checks = append(r.Checks, ReadinessCheck{
		ID:     "repository",
		Label:  "Repository",
		Value:  filepath.Base(u.projectRoot),
		Detail: u.projectRoot,
		State:  StateOK,
	})

	r.Checks = append(r.Checks, r.checkComposerLock(u.projectRoot))
	r.Checks = append(r.Checks, checkGitClean(ctx, u.projectRoot))

	r.Extensions = discoverExtensions(ctx, u.projectRoot)
	r.Checks = append(r.Checks, ReadinessCheck{
		ID:    "extensions",
		Label: "Composer-managed extensions found",
		Value: fmt.Sprintf("%d", r.ComposerManagedCount()),
		State: StateOK,
	})

	r.Checks = append(r.Checks, checkDeploymentHelper(u.projectRoot))
	r.Checks = append(r.Checks, u.checkTooling(ctx))

	return r
}

// checkComposerLock verifies composer.lock exists and contains a Shopware core
// package, extracting the current version into the Readiness result.
func (r *Readiness) checkComposerLock(projectRoot string) ReadinessCheck {
	check := ReadinessCheck{
		ID:       "composer-lock",
		Label:    "composer.lock available",
		Blocking: true,
	}

	lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		check.State = StateFail
		check.Value = "no"
		check.Detail = "Run composer install first — the lock file records the exact installed versions."
		return check
	}

	core := lock.GetPackage("shopware/core")
	if core == nil {
		check.State = StateFail
		check.Value = "no shopware/core"
		check.Detail = "composer.lock does not contain shopware/core — is this a Shopware project?"
		return check
	}

	current, err := version.NewVersion(strings.TrimPrefix(core.Version, "v"))
	if err != nil {
		check.State = StateFail
		check.Value = core.Version
		check.Detail = "could not parse the installed shopware/core version"
		return check
	}

	r.CurrentVersion = current
	check.State = StateOK
	check.Value = "yes"
	return check
}

func checkGitClean(ctx context.Context, projectRoot string) ReadinessCheck {
	check := ReadinessCheck{
		ID:       "git-clean",
		Label:    "Git working tree clean",
		Blocking: true,
	}

	dirty, isRepo, err := git.IsWorkingTreeDirty(ctx, projectRoot)
	switch {
	case err != nil:
		check.State = StateFail
		check.Value = "unknown"
		check.Detail = err.Error()
	case !isRepo:
		check.State = StateWarn
		check.Blocking = false
		check.Value = "no repository"
		check.Detail = "Without version control you cannot review or revert the upgrade changes."
	case dirty:
		check.State = StateFail
		check.Value = "no"
		check.Detail = "Commit or stash your changes so the upgrade starts from a clean state."
	default:
		check.State = StateOK
		check.Value = "yes"
	}
	return check
}

func checkDeploymentHelper(projectRoot string) ReadinessCheck {
	check := ReadinessCheck{
		ID:    "deployment-helper",
		Label: "Deployment Helper workflow ready",
	}

	composerJSON, err := composer.ReadJson(filepath.Join(projectRoot, "composer.json"))
	if err != nil {
		check.State = StateWarn
		check.Value = "unknown"
		check.Detail = "could not read composer.json: " + err.Error()
		return check
	}

	if _, ok := composerJSON.Require[deploymentHelperPackage]; !ok {
		check.State = StateWarn
		check.Value = "no"
		check.Detail = "shopware/deployment-helper is not required; the wizard adds it during the upgrade."
		return check
	}

	check.State = StateOK
	check.Value = "yes"
	return check
}

// checkTooling verifies PHP and Composer are usable through the environment
// the upgrade will run in — for Docker or Symfony CLI environments the
// executor provides them, so probing the host binaries would report the wrong
// result.
func (u *ProjectUpgrader) checkTooling(ctx context.Context) ReadinessCheck {
	check := ReadinessCheck{
		ID:       "tooling",
		Label:    "PHP and Composer available",
		Blocking: true,
	}

	var missing []string
	if err := u.executor.PHPCommand(ctx, "--version").Run(); err != nil {
		missing = append(missing, "PHP")
	}
	if err := u.executor.ComposerCommand(ctx, "--version").Run(); err != nil {
		missing = append(missing, "Composer")
	}

	if len(missing) == 0 {
		check.State = StateOK
		check.Value = "yes"
		return check
	}

	check.State = StateFail
	check.Value = "no"
	check.Detail = "Missing: " + strings.Join(missing, ", ") +
		" (checked through the " + u.executor.Type() + " environment — make sure it is running)"
	return check
}

// discoverExtensions lists the project's extensions, marking whether each is
// Composer-managed (living in vendor/) or a local extension in custom/.
func discoverExtensions(ctx context.Context, projectRoot string) []InstalledExtension {
	found := extension.FindExtensionsFromProject(logging.DisableLogger(ctx), projectRoot, false)

	vendorDir := filepath.Join(projectRoot, "vendor") + string(filepath.Separator)

	result := make([]InstalledExtension, 0, len(found))
	for _, ext := range found {
		name, err := ext.GetName()
		if err != nil {
			continue
		}

		pkg, _ := ext.GetComposerName()

		ver := ""
		if v, err := ext.GetVersion(); err == nil && v != nil {
			ver = v.String()
		}

		result = append(result, InstalledExtension{
			Name:            name,
			Package:         pkg,
			Path:            ext.GetPath(),
			Version:         ver,
			ComposerManaged: strings.HasPrefix(ext.GetPath(), vendorDir),
		})
	}

	return result
}
