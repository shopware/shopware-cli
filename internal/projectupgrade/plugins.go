package projectupgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// RemoveIncompatiblePlugins drops symlinked custom/plugins/* entries from
// composer.json when their declared Shopware constraint is not satisfied by
// targetVersion. Composer would otherwise fail the update because the plugin
// pins us to an older shopware/core. Mirrors PluginCompatibility from the
// shopware/web-installer.
//
// Returns the list of removed plugin names so the caller can report what was
// removed.
func RemoveIncompatiblePlugins(composerJsonPath, targetVersion string) ([]string, error) {
	projectDir := filepath.Dir(composerJsonPath)

	installedPath := filepath.Join(projectDir, "vendor", "composer", "installed.json")

	data, err := os.ReadFile(installedPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
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

	incompatible := make([]string, 0)

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

		incompatible = append(incompatible, pkg.Name)
	}

	if len(incompatible) == 0 {
		return nil, nil
	}

	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return nil, err
	}

	removed := make([]string, 0, len(incompatible))

	for _, name := range incompatible {
		if _, ok := composerJson.Require[name]; ok {
			delete(composerJson.Require, name)
			removed = append(removed, name)
		}
	}

	if len(removed) == 0 {
		return nil, nil
	}

	if err := composerJson.Save(); err != nil {
		return nil, err
	}

	return removed, nil
}

func isInstalledUnderCustomPlugins(projectDir, installPath string) bool {
	if installPath == "" {
		return false
	}

	// install-path is recorded relative to vendor/composer.
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

	if rel == "." || rel == "" {
		return false
	}

	if strings.HasPrefix(rel, "..") {
		return false
	}

	// Direct child of custom/plugins (a single plugin directory).
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
