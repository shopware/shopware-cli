package verifier

import (
	"context"
	"os"
	"path"
	"sort"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/extension"
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

	if err := determineVersionRange(cfg, constraint); err != nil {
		return nil, err
	}

	return cfg, nil
}

func determineVersionRange(cfg *ToolConfig, versionConstraint *version.Constraints) error {
	versions, err := extension.GetShopwareVersions(context.Background())
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

	matchingVersions := make([]*version.Version, 0)

	for _, v := range vs {
		if versionConstraint.Check(v) {
			matchingVersions = append(matchingVersions, v)
		}
	}

	if len(matchingVersions) == 0 {
		matchingVersions = append(matchingVersions, version.Must(version.NewVersion("6.7.0.0")))
	}

	cfg.MinShopwareVersion = matchingVersions[0].String()
	cfg.MaxShopwareVersion = matchingVersions[len(matchingVersions)-1].String()

	return nil
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
