package extension

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

type ExtensionType string

const (
	Plugin ExtensionType = "plugin"
	Theme  ExtensionType = "theme"
)

// CreateOptions contains the choices used to create extension scaffolding.
type CreateOptions struct {
	Name   string
	Vendor string
	Type   ExtensionType
	Store  bool
}

// Create writes extension scaffolding in the closest Shopware project.
func Create(ctx context.Context, opts CreateOptions) (err error) {
	logger := logging.FromContext(ctx)

	logger.Info("Creating extension...")

	projectDir, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return err
	}

	technicalName := deriveTechnicalName(opts.Name, opts.Vendor)
	extensionDir := deriveExtensionDirectoryName(projectDir, opts.Store, technicalName)

	err = scaffolding.CreateExtensionDir(extensionDir)
	if err != nil {
		return err
	}

	// Remove only the directory created above if a later step fails.
	defer func() {
		if err == nil {
			return
		}
		logger.Debugf("Rollback of %s", extensionDir)
		if cleanupErr := scaffolding.RemoveCreatedExtensionDir(extensionDir); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("rollback failed: %w", cleanupErr))
		}
	}()

	if err = scaffolding.CreateExtensionFiles(extensionDir, opts.Name, opts.Vendor); err != nil {
		return fmt.Errorf("create extension files: %w", err)
	}

	logger.Infof("✓ Extension successfully created in %s", extensionDir)

	return nil
}

func deriveTechnicalName(name, vendor string) string {
	if vendor == "" {
		return name
	}
	return vendor + name
}

func deriveExtensionDirectoryName(projectDir string, store bool, technicalName string) string {
	pluginDir := "static-plugins"
	if store {
		pluginDir = "plugins"
	}

	return filepath.Join(projectDir, "custom", pluginDir, technicalName)
}
