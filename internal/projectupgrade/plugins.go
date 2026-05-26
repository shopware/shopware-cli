package projectupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/packagist"
)

// composerPluginType is the composer "type" used by Shopware platform plugins.
const composerPluginType = "shopware-platform-plugin"

// pluginShopwarePackages are the Shopware first-party packages a plugin can
// declare a constraint against. If any constraint cannot be satisfied by the
// target version, the plugin is considered incompatible.
var pluginShopwarePackages = []string{
	"shopware/core",
	"shopware/administration",
	"shopware/storefront",
	"shopware/elasticsearch",
}

type installedPackage struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Require     map[string]string `json:"require"`
	InstallPath string            `json:"install-path"`
}

type installedJSON struct {
	Packages []installedPackage `json:"packages"`
}

// PluginAction describes how the resolver dealt with one incompatible plugin.
type PluginAction struct {
	// Name is the composer package name (e.g. "store.shopware.com/swagcms").
	Name string
	// OldConstraint is the constraint that was in composer.json before
	// resolution.
	OldConstraint string
	// NewConstraint is the constraint that was written to composer.json.
	// Empty when Removed is true.
	NewConstraint string
	// NewVersion is the package version the new constraint points at.
	// Empty when Removed is true.
	NewVersion string
	// Removed is true when no compatible version could be found and the
	// plugin was dropped from composer.json.
	Removed bool
	// Reason is a short human-readable explanation surfaced in the UI.
	Reason string
}

// ResolveResult summarises the actions the resolver took.
type ResolveResult struct {
	Actions []PluginAction
}

// Bumped returns the actions that resulted in a constraint bump.
func (r *ResolveResult) Bumped() []PluginAction {
	out := make([]PluginAction, 0, len(r.Actions))
	for _, a := range r.Actions {
		if !a.Removed {
			out = append(out, a)
		}
	}
	return out
}

// Removed returns the actions that resulted in the plugin being dropped.
func (r *ResolveResult) Removed() []PluginAction {
	out := make([]PluginAction, 0, len(r.Actions))
	for _, a := range r.Actions {
		if a.Removed {
			out = append(out, a)
		}
	}
	return out
}

// ResolveIncompatiblePlugins inspects every shopware platform plugin under
// custom/plugins/* (as listed in vendor/composer/installed.json). For each
// plugin whose installed Shopware constraint is not satisfied by
// targetVersion the resolver tries to find a newer release on the supplied
// registry; if one exists, the composer.json constraint is bumped to
// "^<that-version>". When no compatible version is available the plugin is
// removed from composer.json so composer update doesn't fail.
//
// registry may be nil, in which case every incompatible plugin is removed
// (the previous behaviour).
func ResolveIncompatiblePlugins(ctx context.Context, composerJsonPath, targetVersion string, registry Registry) (*ResolveResult, error) {
	projectDir := filepath.Dir(composerJsonPath)

	installedPath := filepath.Join(projectDir, "vendor", "composer", "installed.json")

	data, err := os.ReadFile(installedPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &ResolveResult{}, nil
		}
		return nil, fmt.Errorf("read installed.json: %w", err)
	}

	var installed installedJSON
	if err := json.Unmarshal(data, &installed); err != nil {
		return nil, fmt.Errorf("parse installed.json: %w", err)
	}

	target, err := version.NewVersion(strings.TrimPrefix(targetVersion, "v"))
	if err != nil {
		return nil, fmt.Errorf("parse target version: %w", err)
	}

	incompatible := make([]installedPackage, 0)
	for _, pkg := range installed.Packages {
		if pkg.Type != composerPluginType {
			continue
		}
		if !isInstalledUnderCustomPlugins(projectDir, pkg.InstallPath) {
			continue
		}
		if pluginSatisfies(pkg.Require, target) {
			continue
		}
		incompatible = append(incompatible, pkg)
	}

	if len(incompatible) == 0 {
		return &ResolveResult{}, nil
	}

	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return nil, err
	}

	result := &ResolveResult{}

	for _, pkg := range incompatible {
		old, ok := composerJson.Require[pkg.Name]
		if !ok {
			continue
		}

		action := PluginAction{Name: pkg.Name, OldConstraint: old}

		newVersion, err := findCompatibleVersion(ctx, registry, pkg.Name, target)
		if err != nil || newVersion == "" {
			delete(composerJson.Require, pkg.Name)
			action.Removed = true
			action.Reason = "no compatible release found"
			if err != nil && !errors.Is(err, ErrRegistryUnavailable) {
				action.Reason = "registry lookup failed: " + err.Error()
			}
			result.Actions = append(result.Actions, action)
			continue
		}

		newConstraint := bumpConstraint(newVersion)
		composerJson.Require[pkg.Name] = newConstraint
		action.NewConstraint = newConstraint
		action.NewVersion = newVersion
		action.Reason = fmt.Sprintf("bumped to %s", newConstraint)
		result.Actions = append(result.Actions, action)
	}

	if len(result.Actions) == 0 {
		return result, nil
	}

	if err := composerJson.Save(); err != nil {
		return nil, err
	}
	return result, nil
}

func findCompatibleVersion(ctx context.Context, registry Registry, name string, target *version.Version) (string, error) {
	if registry == nil {
		return "", ErrRegistryUnavailable
	}

	versions, err := registry.GetPackageVersions(ctx, name)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", nil
	}

	parsed := make([]packagist.ComposerPackageVersion, 0, len(versions))
	for _, v := range versions {
		if isPreReleaseVersion(v.Version) {
			continue
		}
		if !satisfiesShopwareTarget(v.Require, target) {
			continue
		}
		parsed = append(parsed, v)
	}

	if len(parsed) == 0 {
		return "", nil
	}

	sort.Slice(parsed, func(i, j int) bool {
		vi, errI := version.NewVersion(strings.TrimPrefix(parsed[i].Version, "v"))
		vj, errJ := version.NewVersion(strings.TrimPrefix(parsed[j].Version, "v"))
		if errI != nil || errJ != nil {
			return parsed[i].Version > parsed[j].Version
		}
		return vi.GreaterThan(vj)
	})

	return strings.TrimPrefix(parsed[0].Version, "v"), nil
}

func isPreReleaseVersion(v string) bool {
	lower := strings.ToLower(v)
	for _, marker := range []string{"-rc", "-beta", "-alpha", "-dev"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func satisfiesShopwareTarget(requires map[string]string, target *version.Version) bool {
	if len(requires) == 0 {
		// No shopware/core constraint declared — assume compatible.
		return true
	}
	for dep, constraint := range requires {
		if !containsString(pluginShopwarePackages, dep) {
			continue
		}
		c, err := version.NewConstraint(constraint)
		if err != nil {
			return false
		}
		if !c.Check(target) {
			return false
		}
	}
	return true
}

// bumpConstraint converts a concrete version (e.g. "2.3.4") into a caret
// constraint ("^2.3.4") suitable for composer.json. Versions that already
// look like a constraint are passed through unchanged.
func bumpConstraint(version string) string {
	if version == "" {
		return version
	}
	if strings.ContainsAny(version, "^~><*|, ") {
		return version
	}
	return "^" + version
}

// FindNonComposerPlugins returns directories under custom/plugins/ that are
// not tracked by composer (no entry in vendor/composer/installed.json).
// Returns an empty slice when no installed.json is present.
func FindNonComposerPlugins(projectRoot string) ([]string, error) {
	customPlugins := filepath.Join(projectRoot, "custom", "plugins")
	entries, err := os.ReadDir(customPlugins)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", customPlugins, err)
	}

	installedPath := filepath.Join(projectRoot, "vendor", "composer", "installed.json")
	composerTracked := make(map[string]struct{})
	if data, err := os.ReadFile(installedPath); err == nil {
		var installed installedJSON
		if jsonErr := json.Unmarshal(data, &installed); jsonErr == nil {
			for _, pkg := range installed.Packages {
				if dir, ok := installedDirName(projectRoot, pkg.InstallPath); ok {
					composerTracked[dir] = struct{}{}
				}
			}
		}
	}

	orphans := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if _, tracked := composerTracked[entry.Name()]; tracked {
			continue
		}
		orphans = append(orphans, entry.Name())
	}

	sort.Strings(orphans)
	return orphans, nil
}

func installedDirName(projectRoot, installPath string) (string, bool) {
	if installPath == "" {
		return "", false
	}
	abs := installPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, "vendor", "composer", installPath)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		resolved = filepath.Clean(abs)
	}
	customPlugins := filepath.Join(projectRoot, "custom", "plugins")
	resolvedCustom, err := filepath.EvalSymlinks(customPlugins)
	if err != nil {
		resolvedCustom = filepath.Clean(customPlugins)
	}
	rel, err := filepath.Rel(resolvedCustom, resolved)
	if err != nil {
		return "", false
	}
	if rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return "", false
	}
	if strings.ContainsRune(rel, filepath.Separator) {
		return "", false
	}
	return rel, true
}

func isInstalledUnderCustomPlugins(projectDir, installPath string) bool {
	if installPath == "" {
		return false
	}
	absPath := installPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(projectDir, "vendor", "composer", installPath)
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		resolved = filepath.Clean(absPath)
	}
	customPlugins := filepath.Join(projectDir, "custom", "plugins")
	resolvedCustom, err := filepath.EvalSymlinks(customPlugins)
	if err != nil {
		resolvedCustom = filepath.Clean(customPlugins)
	}
	rel, err := filepath.Rel(resolvedCustom, resolved)
	if err != nil {
		return false
	}
	if rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return false
	}
	return !strings.ContainsRune(rel, filepath.Separator)
}

func pluginSatisfies(requires map[string]string, target *version.Version) bool {
	for dep, constraint := range requires {
		if !containsString(pluginShopwarePackages, dep) {
			continue
		}
		c, err := version.NewConstraint(constraint)
		if err != nil {
			continue
		}
		if !c.Check(target) {
			return false
		}
	}
	return true
}

func containsString(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
