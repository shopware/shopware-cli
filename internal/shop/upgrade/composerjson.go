package upgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

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
func applyTargetConstraints(c *composer.Json, target string, extensionPackages []string, resolved map[string]string, pathInstalled map[string]bool) []string {
	var changes []string

	for _, pkg := range shopwarePlatformPackages {
		if c.Require == nil {
			break
		}
		current, ok := c.Require[pkg]
		if !ok || current == target {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s -> %s", pkg, current, target))
		c.Require[pkg] = target
	}

	for _, pkg := range extensionPackages {
		if c.Require == nil {
			break
		}
		current, ok := c.Require[pkg]
		if !ok {
			continue
		}
		// Path-repository packages stay at their local files; opening the
		// constraint to "*" cannot discover a newer published release.
		if pathInstalled[pkg] {
			continue
		}
		constraint := "*"
		if to := resolved[pkg]; to != "" {
			constraint = to
		}
		if current == constraint {
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s -> %s", pkg, current, constraint))
		c.Require[pkg] = constraint
	}

	if !c.HasPackage(deploymentHelperPackage) && !c.HasPackageDev(deploymentHelperPackage) {
		c.AddPackage(deploymentHelperPackage, "*")
		changes = append(changes, deploymentHelperPackage+": added")
	}

	return changes
}

// extensionPackages lists the Composer-managed Shopware extensions
// (plugins, apps, bundles) recorded in composer.lock.
func extensionPackages(projectRoot string) ([]string, error) {
	lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return nil, fmt.Errorf("read composer.lock: %w", err)
	}

	var packages []string
	for _, pkg := range lock.Packages {
		switch pkg.Type {
		case extension.ComposerTypePlugin, extension.ComposerTypeApp, extension.ComposerTypeBundle:
			packages = append(packages, pkg.Name)
		}
	}
	return packages, nil
}

// pathInstalledPackageNames lists Composer packages installed from a path
// repository (composer.lock dist.type == "path").
func pathInstalledPackageNames(projectRoot string) map[string]bool {
	info := lockExtensionInfo(projectRoot)
	if len(info) == 0 {
		return nil
	}
	names := make(map[string]bool, len(info))
	for name, ext := range info {
		if ext.pathInstalled {
			names[name] = true
		}
	}
	return names
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

	extensions, err := extensionPackages(u.projectRoot)
	if err != nil {
		return nil, err
	}

	changes := applyTargetConstraints(c, target, extensions, resolved, pathInstalledPackageNames(u.projectRoot))
	if err := c.Save(); err != nil {
		return nil, err
	}
	return changes, nil
}

// SuggestComposerName derives a valid Composer package name from the OS
// username and the project directory name, e.g. "shyim/acme-shop" for
// "/srv/shops/Acme Shop". It falls back to the "shopware" vendor when no
// usable username is available. It is the pre-filled value of the wizard's
// package-name prompt.
func SuggestComposerName(projectRoot string) string {
	name := sanitizeComposerPart(filepath.Base(projectRoot))
	if name == "" {
		name = "production"
	}
	return defaultComposerVendor() + "/" + name
}

// defaultComposerVendor returns the OS username sanitized into a valid Composer
// vendor, falling back to "shopware" when it cannot be determined or contains
// no usable characters.
func defaultComposerVendor() string {
	if u, err := user.Current(); err == nil && u != nil {
		if v := sanitizeComposerPart(u.Username); v != "" {
			return v
		}
	}
	for _, env := range []string{"USER", "LOGNAME", "USERNAME"} {
		if v := sanitizeComposerPart(os.Getenv(env)); v != "" {
			return v
		}
	}
	return "shopware"
}

// sanitizeComposerPart lowers s and collapses every run of characters outside
// Composer's allowed set into a single dash, trimming leading/trailing dashes
// so the result fits the package-name pattern. It returns "" when nothing
// usable remains.
func sanitizeComposerPart(s string) string {
	s = strings.ToLower(s)

	var b strings.Builder
	separator := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			separator = false
			continue
		}
		// Collapse every run of invalid characters into a single dash; a
		// leading dash would violate the package-name pattern.
		if !separator && b.Len() > 0 {
			b.WriteByte('-')
			separator = true
		}
	}

	return strings.Trim(b.String(), "-")
}

// SetComposerName validates name against Composer's package-name rule and
// writes it into the project's composer.json. The upgrade refuses to run
// without one (see the composer-name readiness check), so the wizard offers
// to set it in place.
func (u *ProjectUpgrader) SetComposerName(name string) error {
	if err := ValidateComposerName(name); err != nil {
		return err
	}

	c, err := composer.ReadJson(filepath.Join(u.projectRoot, "composer.json"))
	if err != nil {
		return fmt.Errorf("read composer.json: %w", err)
	}

	c.Name = name
	if err := c.Save(); err != nil {
		return fmt.Errorf("write composer.json: %w", err)
	}
	return nil
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

	extensions, err := extensionPackages(u.projectRoot)
	if err != nil {
		return nil, err
	}
	applyTargetConstraints(c, target, extensions, nil, pathInstalledPackageNames(u.projectRoot))

	out, err := json.MarshalIndent(c, "", "    ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
