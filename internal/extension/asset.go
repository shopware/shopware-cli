package extension

import (
	"context"
	"os"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/logging"
)

func ConvertExtensionsToSources(ctx context.Context, extensions []Extension) []asset.Source {
	sources := make([]asset.Source, 0)

	for _, ext := range extensions {
		name, err := ext.GetName()
		if err != nil {
			logging.FromContext(ctx).Errorf("Skipping extension %s as it has a invalid name", ext.GetPath())
			continue
		}

		sources = append(sources, asset.Source{
			Name:                        name,
			Path:                        ext.GetRootDir(),
			AdminEsbuildCompatible:      ext.GetExtensionConfig().Build.Zip.Assets.EnableESBuildForAdmin,
			StorefrontEsbuildCompatible: ext.GetExtensionConfig().Build.Zip.Assets.EnableESBuildForStorefront,
			DisableSass:                 ext.GetExtensionConfig().Build.Zip.Assets.DisableSass,
			NpmStrict:                   ext.GetExtensionConfig().Build.Zip.Assets.NpmStrict,
		})

		extConfig := ext.GetExtensionConfig()

		if extConfig != nil {
			for _, bundle := range extConfig.Build.ExtraBundles {
				bundleName := bundle.ResolveName()
				bundlePath := bundle.ResolvePath(ext.GetRootDir())

				if _, err := os.Stat(bundlePath); os.IsNotExist(err) {
					logging.FromContext(ctx).Errorf("Skipping extra bundle %s as its folder %s does not exist", bundleName, bundlePath)
					continue
				}

				sources = append(sources, asset.Source{
					Name:                        bundleName,
					Path:                        bundlePath,
					AdminEsbuildCompatible:      ext.GetExtensionConfig().Build.Zip.Assets.EnableESBuildForAdmin,
					StorefrontEsbuildCompatible: ext.GetExtensionConfig().Build.Zip.Assets.EnableESBuildForStorefront,
					DisableSass:                 ext.GetExtensionConfig().Build.Zip.Assets.DisableSass,
					NpmStrict:                   ext.GetExtensionConfig().Build.Zip.Assets.NpmStrict,
				})
			}
		}
	}

	return sources
}
