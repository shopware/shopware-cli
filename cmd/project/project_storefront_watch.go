package project

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectStorefrontWatchCmd = &cobra.Command{
	Use:     "storefront-watch [path]",
	Short:   "Starts the Shopware Storefront Watcher",
	Aliases: []string{"watch-storefront"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectRoot string
		var err error

		if len(args) == 1 {
			projectRoot = args[0]
		} else if projectRoot, err = findClosestShopwareProject(); err != nil {
			return err
		}

		if err := envfile.LoadSymfonyEnvFile(projectRoot); err != nil {
			return err
		}

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		if err := filterAndWritePluginJson(cmd, projectRoot, shopCfg, cmdExecutor); err != nil {
			return err
		}

		var opts extension.StorefrontWatcherOptions
		if cmd.PersistentFlags().Changed("sales-channel") {
			salesChannelID, _ := cmd.PersistentFlags().GetString("sales-channel")
			opts, err = resolveStorefrontWatcherOptions(cmd.Context(), cmdExecutor, salesChannelID)
			if err != nil {
				return err
			}
		}

		watchProcess, err := extension.PrepareStorefrontWatcher(cmd.Context(), projectRoot, cmdExecutor, opts)
		if err != nil {
			return err
		}

		runErr := runTransparentCommand(watchProcess)

		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = watchProcess.Stop(stopCtx)

		return runErr
	},
}

func init() {
	projectRootCmd.AddCommand(projectStorefrontWatchCmd)
	projectStorefrontWatchCmd.PersistentFlags().String("only-extensions", "", "Only watch the given extensions (comma separated)")
	projectStorefrontWatchCmd.PersistentFlags().String("skip-extensions", "", "Skips the given extensions (comma separated)")
	projectStorefrontWatchCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Only build extensions from custom/static-plugins directory")
	projectStorefrontWatchCmd.PersistentFlags().String("sales-channel", "", "Sales channel ID to target with theme:dump. Pass without a value (--sales-channel) to pick interactively. Omit the flag entirely to keep the legacy theme:dump behavior")
	projectStorefrontWatchCmd.PersistentFlags().Lookup("sales-channel").NoOptDefVal = " "
}

// resolveStorefrontWatcherOptions picks the theme + domain that theme:dump should target.
// Only invoked when the user explicitly passes --sales-channel; an empty salesChannelID
// (flag present without a value) triggers an interactive picker.
func resolveStorefrontWatcherOptions(ctx context.Context, cmdExecutor executor.Executor, salesChannelID string) (extension.StorefrontWatcherOptions, error) {
	salesChannelID = strings.TrimSpace(salesChannelID)

	client, err := cmdExecutor.AdminAPIClient(ctx)
	if err != nil {
		return extension.StorefrontWatcherOptions{}, fmt.Errorf("--sales-channel requires admin api access (set admin_api in .shopware-project.yml or SHOPWARE_CLI_API_* env vars): %w", err)
	}

	apiCtx := adminSdk.NewApiContext(ctx)
	channels, err := client.SalesChannel.ListStorefront(apiCtx)
	if err != nil {
		return extension.StorefrontWatcherOptions{}, fmt.Errorf("listing storefront sales channels: %w", err)
	}

	if len(channels) == 0 {
		return extension.StorefrontWatcherOptions{}, fmt.Errorf("no storefront sales channels found")
	}

	var picked *adminSdk.SalesChannel
	if salesChannelID != "" {
		for i, sc := range channels {
			if sc.Id == salesChannelID {
				picked = &channels[i]
				break
			}
		}
		if picked == nil {
			return extension.StorefrontWatcherOptions{}, fmt.Errorf("sales channel %q not found or not a storefront", salesChannelID)
		}
	} else {
		opts := make([]huh.Option[string], 0, len(channels))
		for _, sc := range channels {
			label := sc.Name
			if len(sc.Domains) > 0 {
				label = fmt.Sprintf("%s (%s)", sc.Name, sc.Domains[0].Url)
			}
			opts = append(opts, huh.NewOption(label, sc.Id))
		}

		var chosenID string
		if err := huh.NewSelect[string]().
			Title("Which sales channel should the storefront watcher target?").
			Options(opts...).
			Value(&chosenID).
			Run(); err != nil {
			return extension.StorefrontWatcherOptions{}, err
		}

		for i, sc := range channels {
			if sc.Id == chosenID {
				picked = &channels[i]
				break
			}
		}
		if picked == nil {
			return extension.StorefrontWatcherOptions{}, fmt.Errorf("no sales channel selected")
		}
	}

	theme, err := client.SalesChannel.FindThemeForSalesChannel(apiCtx, picked.Id)
	if err != nil {
		return extension.StorefrontWatcherOptions{}, fmt.Errorf("resolving theme for sales channel %s: %w", picked.Name, err)
	}
	if theme == nil {
		return extension.StorefrontWatcherOptions{}, fmt.Errorf("no theme assigned to sales channel %s", picked.Name)
	}

	out := extension.StorefrontWatcherOptions{ThemeID: theme.Id}
	if len(picked.Domains) > 0 {
		out.DomainURL = picked.Domains[0].Url
	}
	return out, nil
}
