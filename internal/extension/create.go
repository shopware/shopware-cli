package extension

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/extension/scaffolding"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

type ExtensionType string

const (
	ExtensionTypePlugin ExtensionType = "plugin"
	ExtensionTypeTheme  ExtensionType = "theme"
)

type ExtensionUsage string

const (
	ExtensionUsagePrivate ExtensionUsage = "private"
	ExtensionUsageStore   ExtensionUsage = "store"
)

// CreateOptions holds the options for creating a new extension.
type CreateOptions struct {
	Name             string	
	Usage           ExtensionUsage
	Type            ExtensionType
}

// Create reads the command line arguments and creates a new extension based on the provided parameters.
func Create(ctx context.Context, opts CreateOptions) error {

	logging.FromContext(ctx).Infof("Creating new extension: %s", opts.Name)

	// check if non-interactive, parse and validate arguments

	// ask for extension name if not provided and validate

	// infer namespace

	// ask for extension type (theme or extension)

	// ask if extension for store or private use (show information what this means)

	// ask for compatibility version (>= 6.6)

	// summarize and ask for confirmation
	
	// Create the extensions directory in the Shopware project, in which the extension will live.
	projectDir, err := shop.FindClosestShopwareProject()
	if err != nil {
		return err
	}

	var prefix string = ""
	switch opts.Usage {
		case ExtensionUsagePrivate:
			prefix = "static-"
		case ExtensionUsageStore:
			prefix = ""
	}
	
	extensionDir := filepath.Join(projectDir, "custom", prefix+"plugins", opts.Name)

	logging.FromContext(ctx).Infof("Creating extension directory: %s", extensionDir)

	err = scaffolding.CreateExtensionDir(extensionDir)
	if err != nil {
		return err
	}

	// Create the scaffolding for the new extension inside the extension directory.
	
	// Require the extension directory to exist
	info, err := os.Stat(extensionDir)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("missing extension directory: %s", extensionDir)
	}
	if err != nil {
		return fmt.Errorf("stat extension directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", extensionDir)
	}

	logging.FromContext(ctx).Infof("Creating scaffolding files for extension: %s", opts.Name)
	
	err = scaffolding.CreateScaffoldingFiles(extensionDir, opts.Name)
	if err != nil {
		return err
	}

	// validate the created extension

	// make clear which requiremtents are not checked by the CLI and need manual review

	// inform user if validation is successful
	// make clear that this does not mean that extension will pass store review

	// show possible next steps (maybe hint that lsp helps with development)

	logging.FromContext(ctx).Infof("Extension %s created successfully.", opts.Name)

	return nil
}

// if creation was canceled clean up the created files and directory
func handleCancelation(extensionDir string) error {
	return nil
}