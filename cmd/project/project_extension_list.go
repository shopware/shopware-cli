package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopwareLabs/go-shopware-http-client/extension"
)

var projectExtensionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List available shop extensions with versions and status",
	RunE: func(cmd *cobra.Command, _ []string) error {
		formatName, _ := cmd.Flags().GetString("format")
		outputAsJSON, _ := cmd.Flags().GetBool("json")
		format, err := projectExtensionOutputFormat(formatName, outputAsJSON)
		if err != nil {
			return err
		}

		projectRoot, err := shop.FindClosestShopwareProject(true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		client, err := cmdExecutor.AdminAPIClient(cmd.Context())
		if err != nil {
			return err
		}

		if err := client.ExtensionManager.Refresh(cmd.Context()); err != nil {
			return err
		}

		extensions, err := client.ExtensionManager.ListAvailable(cmd.Context())
		if err != nil {
			return err
		}

		return projectExtensionListTable(extensions).Write(cmd.OutOrStdout(), format)
	},
}

func projectExtensionListTable(extensions extension.List) *tui.Table {
	result := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Version", JSONKey: "version"},
		tui.TableColumn{Title: "Status", JSONKey: "status"},
	)
	for _, ext := range extensions {
		result.AddRowWithJSON(ext, ext.Name, ext.Version, ext.Status())
	}

	return result
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionListCmd)
	projectExtensionListCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table or json)")
	projectExtensionListCmd.Flags().Bool("json", false, "Output as JSON")
	projectExtensionListCmd.MarkFlagsMutuallyExclusive("format", "json")
	_ = projectExtensionListCmd.Flags().MarkDeprecated("json", "use --format json instead")
	_ = projectExtensionListCmd.Flags().MarkHidden("json")
}
