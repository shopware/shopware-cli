package extension

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
)

var extensionVersionCmd = &cobra.Command{
	Use:   "get-version [path]",
	Short: "Get the version of the given extension",
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
			ext, err = extension.GetExtensionByFolder(path)
		} else {
			ext, err = extension.GetExtensionByZip(path)
		}

		if err != nil {
			return fmt.Errorf("version: cannot open extension %w", err)
		}

		version, err := ext.GetVersion()
		if err != nil {
			return fmt.Errorf("cannot generate version: %w", err)
		}

		fmt.Println(version.String())

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionVersionCmd)
}
