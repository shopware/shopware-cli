package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
)

var extensionNameCmd = &cobra.Command{
	Use:   "get-name [path]",
	Short: "Get the name of the given extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		stat, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("cannot find path: %w", err)
		}

		var ext extension.Extension

		if stat.IsDir() {
			ext, err = extension.GetExtensionByFolder(cmd.Context(), path)
		} else {
			ext, err = extension.GetExtensionByZip(path)
		}

		if err != nil {
			return fmt.Errorf("name: cannot open extension %w", err)
		}

		name, err := ext.GetName()
		if err != nil {
			return fmt.Errorf("cannot generate name: %w", err)
		}

		fmt.Println(name)

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionNameCmd)
}
