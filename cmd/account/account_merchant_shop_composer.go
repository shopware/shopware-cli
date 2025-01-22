package account

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

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

			var content []byte

			if content, err = os.ReadFile("composer.json"); err != nil {
				return err
			}

			var composer map[string]interface{}

			if err := json.Unmarshal(content, &composer); err != nil {
				return err
			}

			if _, ok := composer["repositories"]; !ok {
				composer["repositories"] = make(map[string]interface{})
			}

			repositories, ok := composer["repositories"].(map[string]interface{})

			if ok {
				repositories["shopware-packages"] = struct {
					Type string `json:"type"`
					Url  string `json:"url"`
				}{
					Type: "composer",
					Url:  "https://packages.shopware.com",
				}
			} else {
				repositories := composer["repositories"].([]interface{})

				repoExists := false

				for _, repo := range repositories {
					mappedRepo, ok := repo.(map[string]interface{})

					if !ok {
						continue
					}

					if mappedRepo["url"] == "https://packages.shopware.com" {
						repoExists = true
						break
					}
				}

				if !repoExists {
					repositories = append(repositories, map[string]interface{}{
						"type": "composer",
						"url":  "https://packages.shopware.com",
					})

					composer["repositories"] = repositories
				}
			}

			if content, err = json.MarshalIndent(composer, "", "    "); err != nil {
				return err
			}

			if err = os.WriteFile("composer.json", content, os.ModePerm); err != nil {
				return err
			}

			var authJson map[string]interface{}

			if content, err = os.ReadFile("auth.json"); err == nil {
				if err := json.Unmarshal(content, &authJson); err != nil {
					return err
				}
			} else {
				authJson = make(map[string]interface{})
			}

			if _, ok := authJson["bearer"]; !ok {
				authJson["bearer"] = make(map[string]interface{})
			}

			bearer, _ := authJson["bearer"].(map[string]interface{})

			bearer["packages.shopware.com"] = token

			if content, err = json.MarshalIndent(authJson, "", "    "); err != nil {
				return err
			}

			if err = os.WriteFile("auth.json", content, os.ModePerm); err != nil {
				return err
			}
		}

		return nil
	},
}

func init() {
	accountCompanyMerchantShopCmd.AddCommand(accountCompanyMerchantShopComposerCmd)
}
