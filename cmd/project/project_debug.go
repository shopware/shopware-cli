package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/table"
	"github.com/shopware/shopware-cli/logging"
)

var projectDebug = &cobra.Command{
	Use:   "debug",
	Short: "Shows detected Shopware version and detected extensions for further debugging",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		args[0], err = filepath.Abs(args[0])
		if err != nil {
			return err
		}

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		shopwareConstraint, err := extension.GetShopwareProjectConstraint(args[0])
		if err != nil {
			return err
		}

		if shopCfg.IsFallback() {
			fmt.Printf("Could not find a %s, using fallback config\n", projectConfigPath)
		} else {
			fmt.Printf("Found config: Yes\n")
		}
		fmt.Printf("Detected following Shopware version: %s\n", shopwareConstraint.String())

		sources := extension.FindAssetSourcesOfProject(logging.DisableLogger(cmd.Context()), args[0], shopCfg)

		fmt.Println("Following extensions/bundles has been detected")
		table := table.NewWriter(os.Stdout)
		table.Header([]string{"Name", "Path"})

		for _, source := range sources {
			_ = table.Append([]string{source.Name, source.Path})
		}

		_ = table.Render()

		return nil
	},
}

func init() {
	projectRootCmd.AddCommand(projectDebug)
}
