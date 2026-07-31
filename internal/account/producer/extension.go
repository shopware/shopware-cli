package producer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/extension"
)

// ResolveExtension loads an extension from the given path, which can either
// be a folder or a zip file.
func ResolveExtension(ctx context.Context, extensionPath string) (extension.Extension, error) {
	absolutePath, err := filepath.Abs(extensionPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %w", err)
	}

	stat, err := os.Stat(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open file: %w", err)
	}

	if stat.IsDir() {
		return extension.GetExtensionByFolder(ctx, absolutePath)
	}

	return extension.GetExtensionByZip(ctx, absolutePath)
}
