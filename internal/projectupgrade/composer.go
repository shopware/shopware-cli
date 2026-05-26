// Package projectupgrade implements the Shopware project upgrade flow used
// by the `shopware-cli project upgrade` command. The logic mirrors the
// shopware/web-installer Update flow so projects can be upgraded the same
// way from the command line.
package projectupgrade

import (
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/packagist"
)

// ShopwarePackages are the first-party Shopware packages whose constraint is
// rewritten to the new target version in composer.json. This matches the list
// in shopware/web-installer ProjectComposerJsonUpdater.
var ShopwarePackages = []string{
	"shopware/core",
	"shopware/administration",
	"shopware/storefront",
	"shopware/elasticsearch",
}

// UpdateComposerJson rewrites the project composer.json so that composer can
// resolve dependencies for targetVersion. It mirrors the logic of
// `Shopware\WebInstaller\Services\ProjectComposerJsonUpdater`.
func UpdateComposerJson(composerJsonPath, targetVersion string) error {
	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return err
	}

	if isPreRelease(targetVersion) {
		composerJson.MinimumStability = "RC"
	} else {
		composerJson.MinimumStability = ""
	}

	if composerJson.Require == nil {
		composerJson.Require = packagist.ComposerPackageLink{}
	}

	if _, ok := composerJson.Require["symfony/runtime"]; ok {
		composerJson.Require["symfony/runtime"] = ">=5"
	}

	for _, pkg := range ShopwarePackages {
		if _, ok := composerJson.Require[pkg]; ok {
			composerJson.Require[pkg] = targetVersion
		}
	}

	return composerJson.Save()
}

func isPreRelease(targetVersion string) bool {
	v := strings.ToLower(targetVersion)
	return strings.Contains(v, "rc") || strings.Contains(v, "beta") || strings.Contains(v, "alpha")
}

// CurrentShopwareVersion returns the installed Shopware version reported by
// the composer.lock at projectDir. Returns an error when no Shopware package
// is recorded in the lock file.
func CurrentShopwareVersion(projectDir string) (*version.Version, error) {
	lock, err := packagist.ReadComposerLock(projectDir + "/composer.lock")
	if err != nil {
		return nil, err
	}

	for _, name := range []string{"shopware/core", "shopware/platform"} {
		pkg := lock.GetPackage(name)
		if pkg == nil {
			continue
		}

		return version.NewVersion(strings.TrimPrefix(pkg.Version, "v"))
	}

	return nil, errNoShopwareInLock
}
