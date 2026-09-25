package verifier

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

// ConvertExtensionToToolConfig builds the tool config; targetVersion is the raw --target-version input or empty.
func ConvertExtensionToToolConfig(ext extension.Extension, targetVersion string) (*ToolConfig, error) {
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

	if err := determineBaseline(cfg, constraint, targetVersion); err != nil {
		return nil, err
	}

	return cfg, nil
}

// getShopwareVersions returns the available Shopware versions. It is a package
// variable so tests can replace it with a fake that does not hit the network.
var getShopwareVersions = extension.GetShopwareVersions

// determineBaseline resolves the requested target, or picks the lowest stable release matching the constraint.
func determineBaseline(cfg *ToolConfig, versionConstraint *version.Constraints, requested string) error {
	versions, err := getShopwareVersions(context.Background())
	if err != nil {
		return baselineWithoutReleaseList(cfg, versionConstraint, requested, err)
	}

	target := validation.Target{Constraint: constraintString(versionConstraint)}

	if requested != "" {
		resolved, err := ResolveTargetVersion(requested, versions)
		if err != nil {
			return err
		}

		target.Version = resolved
		target.Requested = normalizeRequested(requested)
		target.Source = validation.TargetSourceFlag
		target.WithinConstraint = versionConstraint.Check(version.Must(version.NewVersion(resolved)))
	} else {
		target.Version = lowestMatchingRelease(parseVersions(versions), versionConstraint)
		target.Source = validation.TargetSourceConstraint
		target.WithinConstraint = true

		if target.Version == "" {
			target.Version = "6.7.0.0"
			target.Source = validation.TargetSourceFallback
			target.WithinConstraint = false
		}
	}

	cfg.MinShopwareVersion = target.Version
	cfg.Target = target

	return nil
}

// baselineWithoutReleaseList keeps an exact --target-version usable when Packagist is unreachable.
func baselineWithoutReleaseList(cfg *ToolConfig, versionConstraint *version.Constraints, requested string, cause error) error {
	if requested == "" {
		return cause
	}

	input := normalizeRequested(requested)
	if !isExactRelease(input) {
		return fmt.Errorf("cannot resolve --target-version %q without the Shopware release list: %w. Pass a full release such as 6.7.14.2 to skip the lookup", requested, cause)
	}

	cfg.MinShopwareVersion = input
	cfg.Target = validation.Target{
		Version:          input,
		Requested:        input,
		Source:           validation.TargetSourceFlag,
		Constraint:       constraintString(versionConstraint),
		WithinConstraint: versionConstraint.Check(version.Must(version.NewVersion(input))),
		Unverified:       true,
	}

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
