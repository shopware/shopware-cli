package verifier

import (
	"context"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

func ConvertExtensionToToolConfig(ext extension.Extension) (*ToolConfig, error) {
	var ignores []validation.ToolConfigIgnore

	for _, ignore := range ext.GetExtensionConfig().Validation.Ignore {
		ignores = append(ignores, validation.ToolConfigIgnore{
			Identifier: ignore.Identifier,
			Path:       ignore.Path,
			Message:    ignore.Message,
		})
	}

	cfg := &ToolConfig{
		ToolDirectory:         GetToolDirectory(),
		Extension:             ext,
		ValidationIgnores:     ignores,
		RootDir:               ext.GetPath(),
		SourceDirectories:     ext.GetSourceDirs(),
		AdminDirectories:      getAdminFolders(ext),
		StorefrontDirectories: getStorefrontFolders(ext),
	}

	constraint, err := ext.GetShopwareVersionConstraint()
	if err != nil {
		return nil, err
	}

	if err := determineBaseline(cfg, constraint); err != nil {
		return nil, err
	}

	return cfg, nil
}

// getShopwareVersions returns the available Shopware versions. It is a package
// variable so tests can replace it with a fake that does not hit the network.
var getShopwareVersions = extension.GetShopwareVersions

// determineBaseline picks the lowest stable release matching the constraint and records why.
func determineBaseline(cfg *ToolConfig, versionConstraint *version.Constraints) error {
	versions, err := getShopwareVersions(context.Background())
	if err != nil {
		return err
	}

	vs := make([]*version.Version, 0)

	for _, r := range versions {
		v, err := version.NewVersion(r)
		if err != nil {
			continue
		}

		vs = append(vs, v)
	}

	sort.Sort(version.Collection(vs))

	target := validation.Target{
		Version:          lowestMatchingRelease(vs, versionConstraint),
		Source:           validation.TargetSourceConstraint,
		Constraint:       constraintString(versionConstraint),
		WithinConstraint: true,
	}

	if target.Version == "" {
		target.Version = "6.7.0.0"
		target.Source = validation.TargetSourceFallback
		target.WithinConstraint = false
	}

	cfg.MinShopwareVersion = target.Version
	cfg.Target = target

	return nil
}

// lowestMatchingRelease prefers stable releases; RC and dev tags count only when nothing else matches.
func lowestMatchingRelease(sorted []*version.Version, constraint *version.Constraints) string {
	prerelease := ""

	for _, v := range sorted {
		if !constraint.Check(v) {
			continue
		}

		if !v.IsPrerelease() {
			return v.String()
		}

		if prerelease == "" {
			prerelease = v.String()
		}
	}

	return prerelease
}

// constraintString renders a parsed constraint the way it is written in composer.json.
func constraintString(cs *version.Constraints) string {
	return strings.ReplaceAll(strings.ReplaceAll(cs.String(), "||", " || "), ",", " ")
}

func getAdminFolders(ext extension.Extension) []string {
	paths := []string{}

	for _, sourceDirs := range ext.GetSourceDirs() {
		paths = append(paths, path.Join(sourceDirs, "Resources", "app", "administration"))
	}

	for _, bundle := range ext.GetExtensionConfig().Build.ExtraBundles {
		paths = append(paths, path.Join(ext.GetRootDir(), bundle.Path, "Resources", "app", "administration"))
	}

	return filterNotExistingPaths(paths)
}

func getStorefrontFolders(ext extension.Extension) []string {
	paths := []string{}

	for _, sourceDirs := range ext.GetSourceDirs() {
		paths = append(paths, path.Join(sourceDirs, "Resources", "app", "storefront"))
	}

	for _, bundle := range ext.GetExtensionConfig().Build.ExtraBundles {
		paths = append(paths, path.Join(ext.GetRootDir(), bundle.Path, "Resources", "app", "storefront"))
	}

	return filterNotExistingPaths(paths)
}

func filterNotExistingPaths(paths []string) []string {
	filteredPaths := make([]string, 0)
	for _, p := range paths {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			filteredPaths = append(filteredPaths, p)
		}
	}

	return filteredPaths
}
