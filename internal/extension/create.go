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
	Name  string
	Type  ExtensionType
	Store bool
}

// Create writes extension scaffolding in the closest Shopware project.
func Create(ctx context.Context, opts CreateOptions) (err error) {
	logger := logging.FromContext(ctx)

	projectDir, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return err
	}

	extensionDir := extensionDirectory(projectDir, opts.Store, opts.Name)

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

	if err = scaffolding.CreateExtensionFiles(extensionDir, opts.Name); err != nil {
		return fmt.Errorf("create extension files: %w", err)
	}

	if err = validateCreatedExtension(ctx, extensionDir); err != nil {
		return fmt.Errorf("validate created extension: %w", err)
	}

	logger.Debugf("Extension %s created successfully in %s", opts.Name, extensionDir)

	return nil
}

func extensionDirectory(projectDir string, store bool, name string) string {
	pluginDir := "static-plugins"
	if store {
		pluginDir = "plugins"
	}

	return filepath.Join(projectDir, "custom", pluginDir, name)
}
