package account

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/account-api"
	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/logging"
)

var accountCompanyProducerExtensionUploadCmd = &cobra.Command{
	Use:   "upload [zip]",
	Short: "Uploads a new extension version",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("validate: %w", err)
		}

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return err
		}

		zipExt, err := extension.GetExtensionByZip(path)
		if err != nil {
			return err
		}

		extName, err := zipExt.GetName()
		if err != nil {
			return err
		}

		ext, err := p.GetExtensionByName(cmd.Context(), extName)
		if err != nil {
			return err
		}

		binaries, err := p.GetExtensionBinaries(cmd.Context(), ext.Id)
		if err != nil {
			return err
		}

		zipVersion, err := zipExt.GetVersion()
		if err != nil {
			return err
		}

		var foundBinary *account_api.ExtensionBinary

		for _, binary := range binaries {
			if binary.Version == zipVersion.String() {
				foundBinary = binary
				break
			}
		}

		changelog, err := zipExt.GetChangelog()
		if err != nil {
			return err
		}

		avaiableVersions, err := p.GetSoftwareVersions(cmd.Context(), zipExt.GetType())
		if err != nil {
			return err
		}

		constraint, err := zipExt.GetShopwareVersionConstraint()
		if err != nil {
			return err
		}

		if foundBinary == nil {
			create := account_api.ExtensionCreate{
				Version:          zipVersion.String(),
				SoftwareVersions: avaiableVersions.FilterOnVersionStringList(constraint),
				Changelogs: []account_api.ExtensionUpdateChangelog{
					{Locale: "de_DE", Text: changelog.German},
					{Locale: "en_GB", Text: changelog.English},
				},
			}

			foundBinary, err = p.CreateExtensionBinary(cmd.Context(), ext.Id, create)
			if err != nil {
				return fmt.Errorf("create extension binary: %w", err)
			}

			logging.FromContext(cmd.Context()).Infof("Created new binary with version %s", zipVersion)
		} else {
			logging.FromContext(cmd.Context()).Infof("Found a zip with version %s already. Updating it", zipVersion)
		}

		update := account_api.ExtensionUpdate{
			Id:               foundBinary.Id,
			SoftwareVersions: avaiableVersions.FilterOnVersionStringList(constraint),
			Changelogs: []account_api.ExtensionUpdateChangelog{
				{Locale: "de_DE", Text: changelog.German},
				{Locale: "en_GB", Text: changelog.English},
			},
		}

		err = p.UpdateExtensionBinaryInfo(cmd.Context(), ext.Id, update)
		if err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Updated changelog. Uploading now the zip to remote")

		err = p.UpdateExtensionBinaryFile(cmd.Context(), ext.Id, foundBinary.Id, path)
		if err != nil {
			if strings.Contains(err.Error(), "BinariesException-40") {
				logging.FromContext(cmd.Context()).Infof("Binary version is already published. Skipping upload")
				return nil
			}
		}

		logging.FromContext(cmd.Context()).Infof("Submitting code review request")

		beforeReviews, err := p.GetBinaryReviewResults(cmd.Context(), ext.Id, foundBinary.Id)
		if err != nil {
			return err
		}

		err = p.TriggerCodeReview(cmd.Context(), ext.Id)
		if err != nil {
			return err
		}

		if !skipWaitingForCodereviewResult {
			logging.FromContext(cmd.Context()).Infof("Waiting for code review result")

			time.Sleep(10 * time.Second)

			maxTries := 10
			tried := 0
			for {
				reviews, err := p.GetBinaryReviewResults(cmd.Context(), ext.Id, foundBinary.Id)
				if err != nil {
					return err
				}

				// Review has been updated
				if len(reviews) != len(beforeReviews) {
					lastReview := reviews[len(reviews)-1]

					if !lastReview.IsPending() {
						if lastReview.HasPassed() {
							if lastReview.HasWarnings() {
								logging.FromContext(cmd.Context()).Infof("Code review has been passed but with warnings")
								logging.FromContext(cmd.Context()).Infof(lastReview.GetSummary())
							} else {
								logging.FromContext(cmd.Context()).Infof("Code review has been passed without warnings")
							}

							break
						}

						logging.FromContext(cmd.Context()).Fatalln("Code review has not passed", lastReview.GetSummary())
					}
				}

				time.Sleep(15 * time.Second)
				tried++

				if maxTries == tried {
					logging.FromContext(cmd.Context()).Infof("Skipping waiting for code review result as it took too long")
				}
			}
		}

		return nil
	},
}

var skipWaitingForCodereviewResult bool

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionUploadCmd)
	accountCompanyProducerExtensionUploadCmd.Flags().BoolVar(&skipWaitingForCodereviewResult, "skip-for-review-result", false, "Skips waiting for Code review result")
}
