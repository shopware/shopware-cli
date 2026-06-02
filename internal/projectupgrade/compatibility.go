package projectupgrade

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/packagist"
)

// CompatStatus describes the upgrade plan for one composer-managed plugin.
type CompatStatus int

const (
	// CompatCompatible: the installed version already satisfies the target.
	CompatCompatible CompatStatus = iota
	// CompatUpdatable: a newer compatible release exists; the resolver will
	// bump the constraint.
	CompatUpdatable
	// CompatBlocker: no compatible release is published; the resolver will
	// remove the plugin from composer.json so the upgrade can proceed.
	CompatBlocker
	// CompatUnknown: the registry could not be consulted (e.g. no token for a
	// store plugin); the resolver will drop the plugin too, but the user may
	// want to retry with credentials.
	CompatUnknown
)

// IsBlocker reports whether this status means the upgrade will drop the plugin.
func (s CompatStatus) IsBlocker() bool { return s == CompatBlocker || s == CompatUnknown }

// IsUpdatable reports whether this status means the constraint will be bumped.
func (s CompatStatus) IsUpdatable() bool { return s == CompatUpdatable }

// Label returns a short human-readable label.
func (s CompatStatus) Label() string {
	switch s {
	case CompatCompatible:
		return "Compatible"
	case CompatUpdatable:
		return "Update available"
	case CompatBlocker:
		return "No compatible release"
	case CompatUnknown:
		return "Not in registry"
	}
	return ""
}

// PluginCompat is one row of the compatibility preview.
type PluginCompat struct {
	// Name is the composer package name (e.g. "swag/paypal").
	Name string
	// CurrentVersion is the version recorded in installed.json.
	CurrentVersion string
	// NewVersion is the version the resolver would bump to. Populated when
	// Status is CompatUpdatable.
	NewVersion string
	// Status classifies how the resolver will treat this plugin.
	Status CompatStatus
}

// CheckPluginCompatibility consults the registry for every composer-managed
// shopware platform plugin and reports how the upgrade will treat it. The
// composer.json is not modified; this is a dry-run of
// ResolveIncompatiblePlugins so callers can preview the plan before applying
// it.
func CheckPluginCompatibility(ctx context.Context, composerJsonPath, targetVersion string, registry Registry) ([]PluginCompat, error) {
	projectDir := filepath.Dir(composerJsonPath)

	installed, err := packagist.ReadInstalledJson(projectDir)
	if err != nil {
		return nil, err
	}

	composerJson, err := packagist.ReadComposerJson(composerJsonPath)
	if err != nil {
		return nil, err
	}

	target, err := version.NewVersion(strings.TrimPrefix(targetVersion, "v"))
	if err != nil {
		return nil, fmt.Errorf("parse target version: %w", err)
	}

	results := make([]PluginCompat, 0)
	for _, pkg := range installed.Packages {
		if pkg.Type != composerPluginType {
			continue
		}
		if _, ok := composerJson.Require[pkg.Name]; !ok {
			continue
		}

		row := PluginCompat{Name: pkg.Name, CurrentVersion: strings.TrimPrefix(pkg.Version, "v")}

		if packagist.ConstraintsSatisfiedBy(pkg.Require, ShopwarePackages, target) {
			row.Status = CompatCompatible
			results = append(results, row)
			continue
		}

		newVersion, lookupErr := findCompatibleVersion(ctx, registry, pkg.Name, target)
		switch {
		case newVersion != "":
			row.Status = CompatUpdatable
			row.NewVersion = newVersion
		case lookupErr != nil:
			row.Status = CompatUnknown
		default:
			row.Status = CompatBlocker
		}
		results = append(results, row)
	}

	return results, nil
}
