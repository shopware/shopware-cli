package project

import (
	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectExtensionListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all installed extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		formatName, _ := cmd.Flags().GetString("format")
		format, err := tui.ParseTableFormat(formatName)
		if err != nil {
			return err
		}
		outputAsJSON, _ := cmd.Flags().GetBool("json")
		if outputAsJSON {
			format = tui.TableFormatJSON
		}

		projectRoot, err := findClosestShopwareProject(true)
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

		if _, err := client.ExtensionManager.Refresh(adminSdk.NewApiContext(cmd.Context())); err != nil {
			return err
		}

		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(adminSdk.NewApiContext(cmd.Context()))
		if err != nil {
			return err
		}

		result := tui.NewTable(
			tui.TableColumn{Title: "Name", JSONKey: "name"},
			tui.TableColumn{Title: "Version", JSONKey: "version"},
			tui.TableColumn{Title: "Status", JSONKey: "status"},
		)
		for _, extension := range extensions {
			result.AddRow(extension.Name, extension.Version, extension.Status())
		}

		return result.Write(cmd.OutOrStdout(), format)
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionListCmd)
	projectExtensionListCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table or json)")
	projectExtensionListCmd.Flags().Bool("json", false, "Output as json")
	projectExtensionListCmd.MarkFlagsMutuallyExclusive("format", "json")
}
