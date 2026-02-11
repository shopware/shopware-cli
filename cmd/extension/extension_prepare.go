package extension

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
)

var extensionPrepareCmd = &cobra.Command{
	Use:   "prepare [path]",
	Short: "Install Composer dependencies of an extension and delete unnecessary files for zipping",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("path not found: %w", err)
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), path)
		if err != nil {
			return fmt.Errorf("detect extension type: %w", err)
		}

		err = extension.PrepareFolderForZipping(cmd.Context(), path+"/", ext, ext.GetExtensionConfig())
		if err != nil {
			return fmt.Errorf("prepare zip: %w", err)
		}

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionPrepareCmd)
}
