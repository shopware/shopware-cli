package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"

	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

var experimentalCachingEnabled bool

func init() {
	experimentalCachingEnabled = os.Getenv("SHOPWARE_CLI_EXPERIMENTAL_ASSET_CACHING") == "1"
}

func restoreAssetCaches(ctx context.Context, sources ExtensionAssetConfig, assetCfg AssetBuildConfig) error {
	if !experimentalCachingEnabled {
		return nil
	}

	var errgrp errgroup.Group

	for name, source := range sources {
		if source.RequiresBuild() && !slices.Contains(assetCfg.ForceExtensionBuild, name) {
			errgrp.Go(func() error {
				return restoreAssetCache(ctx, source, assetCfg)
			})
		}
	}

	return errgrp.Wait()
}

func storeAssetCaches(ctx context.Context, sources ExtensionAssetConfig, assetCfg AssetBuildConfig) error {
	if !experimentalCachingEnabled {
		return nil
	}

	var errgrp errgroup.Group

	for name, source := range sources {
		if source.RequiresBuild() && !slices.Contains(assetCfg.ForceExtensionBuild, name) {
			errgrp.Go(func() error {
				return storeAssetCache(ctx, source, assetCfg)
			})
		}
	}

	return errgrp.Wait()
}

func restoreAssetCache(ctx context.Context, source *ExtensionAssetConfigEntry, assetCfg AssetBuildConfig) error {
	assetHash, err := source.GetContentHash()

	if err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("sw-cli-%s-%s", assetCfg.ShopwareVersion.String(), assetHash)

	logging.FromContext(ctx).Debugf("Trying to restore cache from key %s", cacheKey)

	if source.Administration.EntryFilePath != nil {
		if err := system.GetDefaultCache().RestoreFolderCache(ctx, cacheKey+"-administration", source.GetOutputAdminPath()); err != nil {
			if errors.Is(err, system.ErrCacheNotFound) {
				return nil
			}

			return err
		}

		logging.FromContext(ctx).Infof("Restored administration assets for %s from cache", source.TechnicalName)

		source.Administration.EntryFilePath = nil
		source.Administration.Webpack = nil
	}

	if source.Storefront.EntryFilePath != nil {
		if err := system.GetDefaultCache().RestoreFolderCache(ctx, cacheKey+"-storefront", source.GetOutputStorefrontPath()); err != nil {
			if errors.Is(err, system.ErrCacheNotFound) {
				return nil
			}

			return err
		}

		logging.FromContext(ctx).Infof("Restored storefront assets for %s from cache", source.TechnicalName)

		source.Storefront.EntryFilePath = nil
		source.Storefront.Webpack = nil
	}

	return nil
}

func storeAssetCache(ctx context.Context, source *ExtensionAssetConfigEntry, assetCfg AssetBuildConfig) error {
	assetHash, err := source.GetContentHash()

	if err != nil {
		return err
	}

	cacheKey := fmt.Sprintf("sw-cli-%s-%s", assetCfg.ShopwareVersion.String(), assetHash)

	logging.FromContext(ctx).Debugf("Trying to store cache to key %s", cacheKey)

	if source.Administration.EntryFilePath != nil {
		if err := system.GetDefaultCache().StoreFolderCache(ctx, cacheKey+"-administration", source.GetOutputAdminPath()); err != nil {
			return err
		}
	}

	if source.Storefront.EntryFilePath != nil {
		if err := system.GetDefaultCache().StoreFolderCache(ctx, cacheKey+"-storefront", source.GetOutputStorefrontPath()); err != nil {
			return err
		}
	}

	return nil
}
