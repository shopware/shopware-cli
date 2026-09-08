package upgrade

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
)

// shopwareConstraintPackages are checked (in order) to find an extension
// release's Shopware version constraint.
var shopwareConstraintPackages = []string{"shopware/core", "shopware/storefront", "shopware/administration", "shopware/elasticsearch"}

// CheckExtensions classifies every discovered extension against the target
// Shopware version, combining Store compatibility metadata with the Composer
// release constraints (Composer being the source of truth). Results are
// ordered most severe first.
func (u *ProjectUpgrader) CheckExtensions(ctx context.Context, current, target *version.Version, extensions []InstalledExtension) []ExtensionResult {
	storeStatus := u.loadStoreStatus(ctx, current, target, extensions)
	storePlugins := u.loadStorePlugins(ctx, target, extensions)
	repos := u.projectRepositories(ctx)

	results := make([]ExtensionResult, 0, len(extensions))
	for _, ext := range extensions {
		res := classifyExtension(ctx, repos, target, ext)
		if store, ok := storeStatus[ext.Name]; ok {
			res.StoreLabel = store.Status.Label
			applyStoreStatus(&res, store)
		} else if ext.ComposerManaged {
			res.StoreLabel = "Not available in Store"
		}
		if plugin, ok := storePlugins[ext.Name]; ok && plugin.StoreLink != "" {
			res.ChangelogURL = plugin.StoreLink
		}
		results = append(results, res)
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Status.Rank() != results[j].Status.Rank() {
			return results[i].Status.Rank() < results[j].Status.Rank()
		}
		return results[i].Extension.Name < results[j].Extension.Name
	})

	return results
}

// loadStoreStatus queries the Shopware Store update check. Store metadata is
// advisory, so failures degrade to an empty map instead of erroring.
func (u *ProjectUpgrader) loadStoreStatus(ctx context.Context, current, target *version.Version, extensions []InstalledExtension) map[string]account_api.UpdateCheckExtensionCompatibility {
	if current == nil || target == nil {
		return nil
	}

	toCheck := make([]account_api.UpdateCheckExtension, 0, len(extensions))
	for _, ext := range extensions {
		if !ext.ComposerManaged || ext.PathInstalled {
			continue
		}
		v := ext.Version
		if v == "" {
			v = "1.0.0"
		}
		toCheck = append(toCheck, account_api.UpdateCheckExtension{Name: ext.Name, Version: v})
	}
	if len(toCheck) == 0 {
		return nil
	}

	updates, err := u.extensionUpdates(ctx, current.String(), target.String(), toCheck)
	if err != nil {
		logging.FromContext(ctx).Debugf("store update check failed: %v", err)
		return nil
	}

	byName := make(map[string]account_api.UpdateCheckExtensionCompatibility, len(updates))
	for _, u := range updates {
		byName[u.Name] = u
	}
	return byName
}

// loadStorePlugins fetches Store listings (including the storefront URL) for
// Composer-managed extensions. Failures degrade to an empty map.
func (u *ProjectUpgrader) loadStorePlugins(ctx context.Context, target *version.Version, extensions []InstalledExtension) map[string]account_api.StorePlugin {
	if target == nil {
		return nil
	}

	names := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		if ext.ComposerManaged && !ext.PathInstalled {
			names = append(names, ext.Name)
		}
	}
	if len(names) == 0 {
		return nil
	}

	plugins, err := u.storePlugins(ctx, "en_GB", target.String(), names)
	if err != nil {
		logging.FromContext(ctx).Debugf("store plugin lookup failed: %v", err)
		return nil
	}

	byName := make(map[string]account_api.StorePlugin, len(plugins))
	for _, plugin := range plugins {
		byName[plugin.Name] = plugin
	}
	return byName
}

// projectRepositories builds the Composer repository set the project itself
// uses (packagist + any configured private repositories with auth.json
// credentials), so release lookups see the same packages Composer does.
func (u *ProjectUpgrader) projectRepositories(ctx context.Context) *repository.Set {
	composerJSON, err := composer.ReadJson(filepath.Join(u.projectRoot, "composer.json"))
	if err != nil {
		logging.FromContext(ctx).Debugf("read composer.json for repositories: %v", err)
		return repository.NewSet(repository.New(repository.PackagistURL, nil))
	}

	auth, err := composer.ReadAuth(filepath.Join(u.projectRoot, "auth.json"))
	if err != nil {
		// An unreadable auth.json must not drop COMPOSER_AUTH credentials.
		auth = &composer.Auth{}
	}
	if mergeErr := auth.MergeEnv(); mergeErr != nil {
		logging.FromContext(ctx).Debugf("merge COMPOSER_AUTH: %v", mergeErr)
	}

	return u.repositories(composerJSON, auth)
}

// classifyExtension determines the Composer-side compatibility of one
// extension with the target version.
func classifyExtension(ctx context.Context, repos *repository.Set, target *version.Version, ext InstalledExtension) ExtensionResult {
	res := ExtensionResult{Extension: ext}

	if !ext.ComposerManaged {
		res.Status = ExtReview
		res.Detail = "Local extension — the wizard does not check custom project extensions. Review it manually."
		return res
	}

	// Path-repository packages have no published release stream. Judge them
	// by the shopware/core constraint in the installed composer.json.
	if ext.PathInstalled {
		return classifyLocalConstraint(ctx, target, ext)
	}

	pkg, client, err := repos.GetPackage(ctx, ext.Package)
	if err != nil {
		// Unpublished / private packages are missing from remote metadata
		// the same way path packages are. Fall back to the installed
		// constraint instead of treating "not on Packagist" as blocked.
		return classifyLocalConstraint(ctx, target, ext)
	}
	if client != nil {
		res.ChangelogURL = changelogURL(client.URL(), ext.Package)
	}

	installedCompatible := false
	sawConstraint := false
	var lowestCompatible *version.Version

	for _, rel := range pkg.Versions {
		constraint := shopwareConstraintOf(rel)
		if constraint == nil {
			continue
		}
		sawConstraint = true
		if !constraint.Check(target) {
			continue
		}

		relVersion, err := version.NewVersion(strings.TrimPrefix(rel.Version, "v"))
		if err != nil {
			continue
		}
		if relVersion.IsPrerelease() {
			continue
		}

		if extVersion, err := version.NewVersion(strings.TrimPrefix(ext.Version, "v")); err == nil && relVersion.Equal(extVersion) {
			installedCompatible = true
		}
		if lowestCompatible == nil || relVersion.LessThan(lowestCompatible) {
			lowestCompatible = relVersion
		}
	}

	switch {
	case installedCompatible:
		res.Status = ExtOK
		res.Available = ext.Version
		res.Detail = "The installed release already supports the selected Shopware version."
	case lowestCompatible != nil:
		res.Status = ExtNeedsUpdate
		res.Available = lowestCompatible.String()
		res.Detail = "A compatible Composer release is available; the upgrade updates the extension automatically."
	case !sawConstraint:
		// No release declares a Shopware constraint (some repositories strip
		// require metadata) — that is "unknown", not "incompatible". The
		// Composer dry run remains the authoritative gate.
		res.Status = ExtReview
		res.Detail = "The repository metadata does not declare Shopware compatibility; the Composer resolution check decides."
	default:
		res.Status = ExtBlocked
		res.Detail = "No released version of this extension is compatible with the selected Shopware version."
	}

	return res
}

// classifyLocalConstraint judges an installed package by the Shopware
// constraint it already declares (lock file, composer.json, or app manifest).
// The installed files stay; only a constraint that excludes the target blocks.
func classifyLocalConstraint(ctx context.Context, target *version.Version, ext InstalledExtension) ExtensionResult {
	res := ExtensionResult{Extension: ext, Available: ext.Version}

	constraint, raw := localShopwareConstraint(ctx, ext)
	switch {
	case constraint == nil:
		res.Status = ExtReview
		res.Detail = "The installed package does not declare Shopware compatibility; the Composer resolution check decides."
	case target != nil && constraint.Check(target):
		res.Status = ExtOK
		res.Detail = "The installed release already supports the selected Shopware version."
	default:
		res.Status = ExtBlocked
		res.Available = ""
		if raw == "" {
			res.Detail = "The installed package does not allow the selected Shopware version."
		} else {
			res.Detail = fmt.Sprintf("The installed package requires %s, which does not allow Shopware %s.", raw, target)
		}
	}
	return res
}

func localShopwareConstraint(ctx context.Context, ext InstalledExtension) (*version.Constraints, string) {
	if c, raw := shopwareConstraintFromRequire(ext.Require); c != nil {
		return c, raw
	}

	if ext.Path != "" {
		if cj, err := composer.ReadJson(filepath.Join(ext.Path, "composer.json")); err == nil {
			if c, raw := shopwareConstraintFromRequire(cj.Require); c != nil {
				return c, raw
			}
		}
		if extObj, err := extension.GetExtensionByFolder(ctx, ext.Path); err == nil {
			if c, err := extObj.GetShopwareVersionConstraint(); err == nil && c != nil {
				return c, c.String()
			}
		}
	}

	return nil, ""
}

// shopwareConstraintOf extracts the Shopware platform constraint of a release.
func shopwareConstraintOf(rel repository.Version) *version.Constraints {
	c, _ := shopwareConstraintFromRequire(rel.Require)
	return c
}

func shopwareConstraintFromRequire(require map[string]string) (*version.Constraints, string) {
	for _, name := range shopwareConstraintPackages {
		raw, ok := require[name]
		if !ok {
			continue
		}
		c, err := version.NewConstraint(raw)
		if err != nil {
			return nil, raw
		}
		return &c, name + " " + raw
	}
	return nil, ""
}

// applyStoreStatus overlays the Store's verdict on the Composer-derived
// result, downgrading or upgrading the classification where the two sources
// disagree.
func applyStoreStatus(res *ExtensionResult, store account_api.UpdateCheckExtensionCompatibility) {
	statusName := strings.ToLower(store.Status.Name + " " + store.Status.Label)

	if strings.Contains(statusName, "deprecat") || strings.Contains(statusName, "discontinu") || strings.Contains(statusName, "replac") {
		res.Status = ExtDeprecated
		res.Detail = "This extension is deprecated and has no planned support for the selected Shopware version."
		return
	}

	// The Store says compatible but Composer disagrees: a misleading label
	// must not let the upgrade continue.
	if !store.Status.IsBlocker() && res.Status == ExtBlocked {
		res.Status = ExtMismatch
		res.Detail = "Store compatibility metadata and Composer constraints do not match. Composer resolution blocks this upgrade."
	}
}

// TargetPHPRequirement returns the PHP constraint the target shopware/core
// release declares (e.g. ">=8.2"), or "" when it cannot be determined.
func (u *ProjectUpgrader) TargetPHPRequirement(ctx context.Context, target *version.Version) string {
	repos := u.projectRepositories(ctx)
	pkg, _, err := repos.GetPackage(ctx, "shopware/core")
	if err != nil {
		return ""
	}

	for _, rel := range pkg.Versions {
		v, err := version.NewVersion(strings.TrimPrefix(rel.Version, "v"))
		if err != nil || !v.Equal(target) {
			continue
		}
		return rel.Require["php"]
	}
	return ""
}

// changelogURL points at the package page of the repository that provides the
// extension. Prefer the Shopware Store listing when CheckExtensions has one.
func changelogURL(repoURL, pkg string) string {
	if strings.Contains(repoURL, "packagist.org") {
		return "https://packagist.org/packages/" + pkg
	}
	return ""
}
