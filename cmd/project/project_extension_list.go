package project

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectExtensionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all installed extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var cfg *shop.Config
		var err error

		outputAsJson, _ := cmd.PersistentFlags().GetBool("json")

		if cfg, err = shop.ReadConfig(cmd.Context(), projectConfigPath, true); err != nil {
			return err
		}

		client, err := shop.NewShopClient(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		if _, err := client.ExtensionManager.Refresh(adminSdk.NewApiContext(cmd.Context())); err != nil {
			return err
		}

		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(adminSdk.NewApiContext(cmd.Context()))
		if err != nil {
			return err
		}

		if outputAsJson {
			content, err := json.Marshal(extensions)
			if err != nil {
				return err
			}

			fmt.Println(string(content))

			return nil
		}

		rows := make([][]string, 0, len(extensions))
		for _, extension := range extensions {
			rows = append(rows, []string{extension.Name, extension.Version, extension.Status()})
		}
		tui.PrintTable([]string{"Name", "Version", "Status"}, rows)

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionListCmd)
	projectExtensionListCmd.PersistentFlags().Bool("json", false, "Output as json")
}
