package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopwareLabs/go-shopware-http-client/extension"
)

var projectExtensionOutdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "List all outdated extensions",
	Long:  "List installed extensions that have a newer version available. Exits with code 1 when at least one extension is outdated, regardless of the output format.",
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
		extensions = extensions.FilterByUpdatable()

		if len(extensions) == 0 && format == tui.TableFormatTable {
			logging.FromContext(cmd.Context()).Infof("All extensions are up-to-date")
			return nil
		}

		result := projectExtensionOutdatedTable(extensions)
		if err := result.Write(cmd.OutOrStdout(), format); err != nil {
			return err
		}
		return fmt.Errorf("there are %d outdated extensions", len(extensions))
	},
}

func projectExtensionOutdatedTable(extensions extension.List) *tui.Table {
	result := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Current Version", JSONKey: "currentVersion"},
		tui.TableColumn{Title: "Latest Version", JSONKey: "latestVersion"},
		tui.TableColumn{Title: "Update Source", JSONKey: "updateSource"},
	)
	for _, ext := range extensions {
		result.AddRowWithJSON(ext, ext.Name, ext.Version, ext.LatestVersion, ext.UpdateSource)
	}

	return result
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionOutdatedCmd)
	projectExtensionOutdatedCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table or json)")
	projectExtensionOutdatedCmd.Flags().Bool("json", false, "Output as JSON")
	projectExtensionOutdatedCmd.MarkFlagsMutuallyExclusive("format", "json")
	_ = projectExtensionOutdatedCmd.Flags().MarkDeprecated("json", "use --format json instead")
	_ = projectExtensionOutdatedCmd.Flags().MarkHidden("json")
}
