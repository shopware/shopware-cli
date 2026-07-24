package upgrade

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/shyim/go-composer"

	"github.com/shopware/shopware-cli/internal/extension"
)

// shopwarePlatformPackages are the packages pinned to the chosen Shopware
// version during the upgrade. Only packages already required by the project
// are touched.
var shopwarePlatformPackages = []string{
	"shopware/core",
	"shopware/administration",
	"shopware/storefront",
	"shopware/elasticsearch",
}

// applyTargetConstraints pins the required Shopware platform packages to the
// target version, rewrites every Composer-managed extension to the version it
// resolved to (falling back to "*" so the solver picks the release matching
// the new platform — the web-installer approach), and makes sure the
// Deployment Helper is required. It returns a human-readable list of the
// changes it made.
func applyTargetConstraints(c *composer.Json, target string, extensionPackages []string, resolved map[string]string) []string {
	var changes []string

	for _, pkg := range shopwarePlatformPackages {
		if !c.HasPackage(pkg) {
			continue
		}
		if c.Require[pkg] == target {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s -> %s", pkg, c.Require[pkg], target))
		c.Require[pkg] = target
	}

	for _, pkg := range extensionPackages {
		if !c.HasPackage(pkg) {
			continue
		}
		constraint := "*"
		if to := resolved[pkg]; to != "" {
			constraint = to
		}
		if c.Require[pkg] == constraint {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s -> %s", pkg, c.Require[pkg], constraint))
		c.Require[pkg] = constraint
	}

	if !c.HasPackage(deploymentHelperPackage) {
		c.AddPackage(deploymentHelperPackage, "*")
		changes = append(changes, deploymentHelperPackage+": added")
	}

	return changes
}

// disableAuditBlock sets config.audit.block-insecure = false so Composer
// (>= 2.9) loads packages affected by security advisories — the same setting
// project creation writes when the user opts out of audit blocking.
func disableAuditBlock(c *composer.Json) {
	audit, _ := c.Config["audit"].(map[string]any)
	if audit == nil {
		audit = map[string]any{}
	}
	audit["block-insecure"] = false
	if c.Config == nil {
		c.Config = map[string]any{}
	}
	c.Config["audit"] = audit
}

// extensionPackages lists the Composer-managed Shopware extensions
// (plugins, apps, bundles) recorded in composer.lock.
func extensionPackages(projectRoot string) []string {
	lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return nil
	}

	var packages []string
	for _, pkg := range lock.Packages {
		switch pkg.Type {
		case extension.ComposerTypePlugin, extension.ComposerTypeApp, extension.ComposerTypeBundle:
			packages = append(packages, pkg.Name)
		}
	}
	return packages
}

// RewriteComposerJSON applies the target constraints to the project's real
// composer.json and saves it, pinning extensions to the versions the
// preflight resolution picked. Used by the runner after the user confirmed
// the plan.
func (u *ProjectUpgrader) RewriteComposerJSON(target string, resolved map[string]string) ([]string, error) {
	c, err := composer.ReadJson(filepath.Join(u.projectRoot, "composer.json"))
	if err != nil {
		return nil, err
	}

	changes := applyTargetConstraints(c, target, extensionPackages(u.projectRoot), resolved)
	if u.noAudit {
		disableAuditBlock(c)
		changes = append(changes, "config.audit.block-insecure: false (continue despite security advisories)")
	}
	if err := c.Save(); err != nil {
		return nil, err
	}
	return changes, nil
}

// renderUpgradeManifest returns the project's composer.json with the target
// constraints applied, without touching any project file. Used for the
// resolution check — extensions stay at "*" here so the solver is free to
// discover the matching releases.
func (u *ProjectUpgrader) renderUpgradeManifest(target string) ([]byte, error) {
	c, err := composer.ReadJson(filepath.Join(u.projectRoot, "composer.json"))
	if err != nil {
		return nil, err
	}

	applyTargetConstraints(c, target, extensionPackages(u.projectRoot), nil)
	if u.noAudit {
		disableAuditBlock(c)
	}

	out, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
