package account

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
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

		logging.FromContext(cmd.Context()).Debugf("Starting upload process for file: %s", path)

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get producer client: %v", err)
			return err
		}

		zipExt, err := extension.GetExtensionByZip(cmd.Context(), path)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to read extension from zip: %v", err)
			return err
		}

		extName, err := zipExt.GetName()
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get extension name: %v", err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Extension name: %s", extName)

		ext, err := p.GetExtensionByName(cmd.Context(), extName)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get extension by name '%s': %v", extName, err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Found extension with ID: %d", ext.Id)

		binaries, err := p.GetExtensionBinaries(cmd.Context(), ext.Id)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get extension binaries for extension ID %d: %v", ext.Id, err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Retrieved %d existing binaries", len(binaries))

		zipVersion, err := zipExt.GetVersion()
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get zip version: %v", err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Zip version: %s", zipVersion.String())

		var foundBinary *account_api.ExtensionBinary

		for _, binary := range binaries {
			if binary.Version == zipVersion.String() {
				foundBinary = binary
				break
			}
		}

		changelog, err := zipExt.GetChangelog()
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get changelog: %v", err)
			return err
		}

		avaiableVersions, err := p.GetSoftwareVersions(cmd.Context(), zipExt.GetType())
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get software versions for type %s: %v", zipExt.GetType(), err)
			return err
		}

		constraint, err := zipExt.GetShopwareVersionConstraint()
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get Shopware version constraint: %v", err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Shopware version constraint: %s", constraint)

		if foundBinary == nil {
			logging.FromContext(cmd.Context()).Debugf("No existing binary found for version %s, creating new one", zipVersion.String())

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
				logging.FromContext(cmd.Context()).Debugf("Failed to create extension binary: %v", err)
				return fmt.Errorf("create extension binary: %w", err)
			}

			logging.FromContext(cmd.Context()).Infof("Created new binary with version %s", zipVersion)
			logging.FromContext(cmd.Context()).Debugf("Created binary with ID: %d", foundBinary.Id)
		} else {
			logging.FromContext(cmd.Context()).Infof("Found a zip with version %s already. Updating it", zipVersion)
			logging.FromContext(cmd.Context()).Debugf("Existing binary ID: %d", foundBinary.Id)
		}

		update := account_api.ExtensionUpdate{
			Id:               foundBinary.Id,
			SoftwareVersions: avaiableVersions.FilterOnVersionStringList(constraint),
			Changelogs: []account_api.ExtensionUpdateChangelog{
				{Locale: "de_DE", Text: changelog.German},
				{Locale: "en_GB", Text: changelog.English},
			},
		}

		logging.FromContext(cmd.Context()).Debugf("Updating extension binary info for extension ID %d, binary ID %d", ext.Id, foundBinary.Id)

		err = p.UpdateExtensionBinaryInfo(cmd.Context(), ext.Id, update)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to update extension binary info: %v", err)
			return err
		}

		logging.FromContext(cmd.Context()).Infof("Updated changelog. Uploading now the zip to remote")
		logging.FromContext(cmd.Context()).Debugf("Uploading zip file from path: %s", path)

		err = p.UpdateExtensionBinaryFile(cmd.Context(), ext.Id, foundBinary.Id, path)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("UpdateExtensionBinaryFile returned error: %v", err)
			if strings.Contains(err.Error(), "BinariesException-40") {
				logging.FromContext(cmd.Context()).Infof("Binary version is already published. Skipping upload")
				return nil
			}
			logging.FromContext(cmd.Context()).Debugf("Error is not BinariesException-40, returning error")
			return fmt.Errorf("upload extension binary file: %w", err)
		}

		logging.FromContext(cmd.Context()).Debugf("Successfully uploaded extension binary file")
		logging.FromContext(cmd.Context()).Infof("Submitting code review request")

		beforeReviews, err := p.GetBinaryReviewResults(cmd.Context(), ext.Id, foundBinary.Id)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get binary review results before triggering review: %v", err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Found %d existing reviews before triggering new review", len(beforeReviews))

		err = p.TriggerCodeReview(cmd.Context(), ext.Id)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to trigger code review: %v", err)
			return err
		}

		logging.FromContext(cmd.Context()).Debugf("Successfully triggered code review")

		if !skipWaitingForCodereviewResult {
			logging.FromContext(cmd.Context()).Infof("Waiting for code review result")
			logging.FromContext(cmd.Context()).Debugf("Initial wait of 10 seconds before first poll")

			time.Sleep(10 * time.Second)

			maxTries := 10
			tried := 0
			for {
				logging.FromContext(cmd.Context()).Debugf("Polling for code review result (attempt %d/%d)", tried+1, maxTries)

				reviews, err := p.GetBinaryReviewResults(cmd.Context(), ext.Id, foundBinary.Id)
				if err != nil {
					logging.FromContext(cmd.Context()).Debugf("Failed to get binary review results during polling: %v", err)
					return err
				}

				logging.FromContext(cmd.Context()).Debugf("Current review count: %d, previous count: %d", len(reviews), len(beforeReviews))

				// Review has been updated
				if len(reviews) != len(beforeReviews) {
					lastReview := reviews[len(reviews)-1]
					logging.FromContext(cmd.Context()).Debugf("Review has been updated, checking status")

					if !lastReview.IsPending() {
						logging.FromContext(cmd.Context()).Debugf("Review is no longer pending")
						if lastReview.HasPassed() {
							if lastReview.HasWarnings() {
								logging.FromContext(cmd.Context()).Infof("Code review has been passed but with warnings")
								logging.FromContext(cmd.Context()).Infof(lastReview.GetSummary())
							} else {
								logging.FromContext(cmd.Context()).Infof("Code review has been passed without warnings")
							}

							break
						}

						logging.FromContext(cmd.Context()).Debugf("Code review failed")
						logging.FromContext(cmd.Context()).Fatalln("Code review has not passed", lastReview.GetSummary())
					} else {
						logging.FromContext(cmd.Context()).Debugf("Review is still pending")
					}
				} else {
					logging.FromContext(cmd.Context()).Debugf("No new reviews yet, waiting...")
				}

				time.Sleep(15 * time.Second)
				tried++

				if maxTries == tried {
					logging.FromContext(cmd.Context()).Infof("Skipping waiting for code review result as it took too long")
					logging.FromContext(cmd.Context()).Debugf("Reached maximum retry attempts (%d)", maxTries)
					break
				}
			}
		} else {
			logging.FromContext(cmd.Context()).Debugf("Skipping code review wait as requested by flag")
		}

		logging.FromContext(cmd.Context()).Debugf("Upload command completed successfully")
		return nil
	},
}

var skipWaitingForCodereviewResult bool

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionUploadCmd)
	accountCompanyProducerExtensionUploadCmd.Flags().BoolVar(&skipWaitingForCodereviewResult, "skip-for-review-result", false, "Skips waiting for Code review result")
}
