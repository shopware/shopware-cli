package project

import (
	"fmt"

	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionOutdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "List all outdated extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		formatName, _ := cmd.Flags().GetString("format")
		outputAsJSON, _ := cmd.Flags().GetBool("json")
		format, err := projectExtensionOutputFormat(formatName, outputAsJSON)
		if err != nil {
			return err
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
		extensions = extensions.FilterByUpdateable()

		if len(extensions) == 0 && format == tui.TableFormatTable {
			logging.FromContext(cmd.Context()).Infof("All extensions are up-to-date")
			return nil
		}

		result := projectExtensionOutdatedTable(extensions)
		if err := result.Write(cmd.OutOrStdout(), format); err != nil {
			return err
		}
		if format == tui.TableFormatJSON {
			return nil
		}

		return fmt.Errorf("there are %d outdated extensions", len(extensions))
	},
}

func projectExtensionOutdatedTable(extensions adminSdk.ExtensionList) *tui.Table {
	result := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Current Version", JSONKey: "currentVersion"},
		tui.TableColumn{Title: "Latest Version", JSONKey: "latestVersion"},
		tui.TableColumn{Title: "Update Source", JSONKey: "updateSource"},
	)
	for _, extension := range extensions {
		result.AddRowWithJSON(extension, extension.Name, extension.Version, extension.LatestVersion, extension.UpdateSource)
	}

	return result
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionOutdatedCmd)
	projectExtensionOutdatedCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table or json)")
	projectExtensionOutdatedCmd.Flags().Bool("json", false, "Output as json")
	projectExtensionOutdatedCmd.MarkFlagsMutuallyExclusive("format", "json")
	_ = projectExtensionOutdatedCmd.Flags().MarkDeprecated("json", "use --format json instead")
	_ = projectExtensionOutdatedCmd.Flags().MarkHidden("json")
}
