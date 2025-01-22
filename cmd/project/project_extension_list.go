package project

import (
	"encoding/json"
	"fmt"
	"os"

	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/shop"
)

var projectExtensionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all installed extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var cfg *shop.Config
		var err error

		outputAsJson, _ := cmd.PersistentFlags().GetBool("json")

		if cfg, err = shop.ReadConfig(projectConfigPath, false); err != nil {
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

		table := tablewriter.NewWriter(os.Stdout)
		table.SetColWidth(100)
		table.SetHeader([]string{"Name", "Version", "Status"})

		for _, extension := range extensions {
			table.Append([]string{extension.Name, extension.Version, extension.Status()})
		}

		table.Render()

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionListCmd)
	projectExtensionListCmd.PersistentFlags().Bool("json", false, "Output as json")
}
