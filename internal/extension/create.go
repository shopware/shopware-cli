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

type ExtensionUsage string

const (
	PrivateUsage ExtensionUsage = "private"
	CommercialUsage   ExtensionUsage = "store"
)

// CreateOptions contains the choices used to create extension scaffolding.
type CreateOptions struct {
	Name  string
	Usage ExtensionUsage
}

// Create writes extension scaffolding in the closest Shopware project.
func Create(ctx context.Context, opts CreateOptions) (err error) {
	projectDir, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return err
	}

	extensionDir := extensionDirectory(projectDir, opts)
	logger := logging.FromContext(ctx)
	logger.Infof("Creating extension %s in %s", opts.Name, extensionDir)

	if err = scaffolding.CreateExtensionDir(extensionDir); err != nil {
		// The target was not created by this process and must remain untouched.
		return err
	}

	// Remove only the directory created above if a later step fails.
	defer func() {
		if err == nil {
			return
		}
		logger.Infof("Removing incomplete extension %s", extensionDir)
		if cleanupErr := scaffolding.RemoveCreatedExtensionDir(extensionDir); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up incomplete extension: %w", cleanupErr))
		}
	}()

	if err = scaffolding.CreateScaffoldingFiles(extensionDir, opts.Name); err != nil {
		return fmt.Errorf("create extension scaffolding: %w", err)
	}

	if err = validateCreatedExtension(ctx, extensionDir); err != nil {
		return fmt.Errorf("validate created extension: %w", err)
	}

	logger.Infof("Extension %s created successfully in %s", opts.Name, extensionDir)
	logger.Infof("The generated plugin is installable; run `shopware-cli extension validate %s` before publishing it", extensionDir)

	return nil
}

func extensionDirectory(projectDir string, opts CreateOptions) string {
	pluginDir := "plugins"
	if opts.Usage == PrivateUsage {
		pluginDir = "static-plugins"
	}

	return filepath.Join(projectDir, "custom", pluginDir, opts.Name)
}
