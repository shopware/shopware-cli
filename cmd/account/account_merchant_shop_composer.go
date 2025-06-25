package account

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/logging"
)

var accountCompanyMerchantShopComposerCmd = &cobra.Command{
	Use:   "configure-composer [domain]",
	Short: "Configure local composer.json to use packages.shopware.com",
	Args:  cobra.MinimumNArgs(1),
	ValidArgsFunction: func(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		completions := make([]string, 0)

		shops, err := services.AccountClient.Merchant().Shops(cmd.Context())
		if err != nil {
			return completions, cobra.ShellCompDirectiveNoFileComp
		}

		for _, shop := range shops {
			completions = append(completions, shop.Domain)
		}

		return completions, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		shops, err := services.AccountClient.Merchant().Shops(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get shops: %w", err)
		}

		shop := shops.GetByDomain(args[0])

		if shop == nil {
			return fmt.Errorf("cannot find shop by domain %s", args[0])
		}

		token, err := services.AccountClient.Merchant().GetComposerToken(cmd.Context(), shop.Id)
		if err != nil {
			return err
		}

		if token == "" {
			generatedToken, err := services.AccountClient.Merchant().GenerateComposerToken(cmd.Context(), shop.Id)
			if err != nil {
				return err
			}

			if err := services.AccountClient.Merchant().SaveComposerToken(cmd.Context(), shop.Id, generatedToken); err != nil {
				return err
			}

			token = generatedToken
		}

		logging.FromContext(cmd.Context()).Infof("The composer token is %s", token)

		if _, err := os.Stat("composer.json"); err == nil {
			logging.FromContext(cmd.Context()).Info("Found composer.json, adding it now as repository")

			composerJson, err := packagist.ReadComposerJson("composer.json")
			if err != nil {
				return err
			}

			if !composerJson.Repositories.HasRepository("https://packages.shopware.com") {
				composerJson.Repositories = append(composerJson.Repositories, packagist.ComposerJsonRepository{
					Type: "composer",
					URL:  "https://packages.shopware.com",
				})
			}

			if err := composerJson.Save(); err != nil {
				return err
			}

			composerAuth, err := packagist.ReadComposerAuth("auth.json")
			if err != nil {
				return err
			}

			composerAuth.BearerAuth["packages.shopware.com"] = token

			if err := composerAuth.Save(); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	accountCompanyMerchantShopCmd.AddCommand(accountCompanyMerchantShopComposerCmd)
}
