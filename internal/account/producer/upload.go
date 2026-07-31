package producer

import (
	"context"
	"fmt"
	"strings"
	"time"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
)

// UploadOptions configures Upload. Use NewUploadOptions for the defaults used
// by the CLI; tests can lower the wait times.
type UploadOptions struct {
	// SkipWaitingForReviewResult skips polling for the automatic code review
	// result after the binary has been uploaded.
	SkipWaitingForReviewResult bool
	// InitialWait is the time to wait before the first code review poll.
	InitialWait time.Duration
	// PollInterval is the time between code review polls.
	PollInterval time.Duration
	// MaxPolls is the number of code review polls before giving up.
	MaxPolls int
}

// NewUploadOptions returns the default upload options.
func NewUploadOptions() UploadOptions {
	return UploadOptions{
		InitialWait:  10 * time.Second,
		PollInterval: 15 * time.Second,
		MaxPolls:     10,
	}
}

// Upload creates or updates the binary of the given extension version in the
// account, uploads the zip file and triggers the automatic code review.
func Upload(ctx context.Context, api UploadAPI, zipExt extension.Extension, zipPath string, opts UploadOptions) error {
	extName, err := zipExt.GetName()
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get extension name: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Extension name: %s", extName)

	ext, err := api.GetExtensionByName(ctx, extName)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get extension by name '%s': %v", extName, err)
		return err
	}

	logging.FromContext(ctx).Debugf("Found extension with ID: %d", ext.Id)

	binaries, err := api.GetExtensionBinaries(ctx, ext.Producer.Id, ext.Id)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get extension binaries for extension ID %d: %v", ext.Id, err)
		return err
	}

	logging.FromContext(ctx).Debugf("Retrieved %d existing binaries", len(binaries))

	zipVersion, err := zipExt.GetVersion()
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get zip version: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Zip version: %s", zipVersion.String())

	var foundBinary *accountApi.ExtensionBinary

	for _, binary := range binaries {
		if binary.Version == zipVersion.String() {
			foundBinary = binary
			break
		}
	}

	changelog, err := zipExt.GetChangelog()
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get changelog: %v", err)
		return err
	}

	avaiableVersions, err := api.GetSoftwareVersions(ctx, zipExt.GetType())
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get software versions for type %s: %v", zipExt.GetType(), err)
		return err
	}

	constraint, err := zipExt.GetShopwareVersionConstraint()
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get Shopware version constraint: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Shopware version constraint: %s", constraint)

	if foundBinary == nil {
		logging.FromContext(ctx).Debugf("No existing binary found for version %s, creating new one", zipVersion.String())

		create := accountApi.ExtensionCreate{
			Version:          zipVersion.String(),
			SoftwareVersions: avaiableVersions.FilterOnVersionStringList(constraint),
			Changelogs: []accountApi.ExtensionUpdateChangelog{
				{Locale: "de_DE", Text: changelog.German},
				{Locale: "en_GB", Text: changelog.English},
			},
		}

		foundBinary, err = api.CreateExtensionBinary(ctx, ext.Producer.Id, ext.Id, create)
		if err != nil {
			logging.FromContext(ctx).Debugf("Failed to create extension binary: %v", err)
			return fmt.Errorf("create extension binary: %w", err)
		}

		logging.FromContext(ctx).Infof("Created new binary with version %s", zipVersion)
		logging.FromContext(ctx).Debugf("Created binary with ID: %d", foundBinary.Id)
	} else {
		logging.FromContext(ctx).Infof("Found a zip with version %s already. Updating it", zipVersion)
		logging.FromContext(ctx).Debugf("Existing binary ID: %d", foundBinary.Id)
	}

	update := accountApi.ExtensionUpdate{
		Id:               foundBinary.Id,
		SoftwareVersions: avaiableVersions.FilterOnVersionStringList(constraint),
		Changelogs: []accountApi.ExtensionUpdateChangelog{
			{Locale: "de_DE", Text: changelog.German},
			{Locale: "en_GB", Text: changelog.English},
		},
	}

	logging.FromContext(ctx).Debugf("Updating extension binary info for extension ID %d, binary ID %d", ext.Id, foundBinary.Id)

	err = api.UpdateExtensionBinaryInfo(ctx, ext.Producer.Id, ext.Id, update)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to update extension binary info: %v", err)
		return err
	}

	logging.FromContext(ctx).Infof("Updated changelog. Uploading now the zip to remote")
	logging.FromContext(ctx).Debugf("Uploading zip file from path: %s", zipPath)

	err = api.UpdateExtensionBinaryFile(ctx, ext.Producer.Id, ext.Id, foundBinary.Id, zipPath)
	if err != nil {
		logging.FromContext(ctx).Debugf("UpdateExtensionBinaryFile returned error: %v", err)
		if strings.Contains(err.Error(), "BinariesException-40") {
			logging.FromContext(ctx).Infof("Binary version is already published. Skipping upload")
			return nil
		}
		logging.FromContext(ctx).Debugf("Error is not BinariesException-40, returning error")
		return fmt.Errorf("upload extension binary file: %w", err)
	}

	logging.FromContext(ctx).Debugf("Successfully uploaded extension binary file")
	logging.FromContext(ctx).Infof("Submitting code review request")

	beforeReviews, err := api.GetBinaryReviewResults(ctx, ext.Id, foundBinary.Id)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get binary review results before triggering review: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Found %d existing reviews before triggering new review", len(beforeReviews))

	err = api.TriggerCodeReview(ctx, ext.Id)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to trigger code review: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Successfully triggered code review")

	if opts.SkipWaitingForReviewResult {
		logging.FromContext(ctx).Debugf("Skipping code review wait as requested by flag")
		logging.FromContext(ctx).Debugf("Upload command completed successfully")
		return nil
	}

	if err := waitForCodeReviewResult(ctx, api, ext.Id, foundBinary.Id, len(beforeReviews), opts); err != nil {
		return err
	}

	logging.FromContext(ctx).Debugf("Upload command completed successfully")
	return nil
}

// waitForCodeReviewResult polls the account API until a new code review
// result appears and is no longer pending, or the maximum number of polls has
// been reached.
func waitForCodeReviewResult(ctx context.Context, api UploadAPI, extensionId, binaryId, knownReviews int, opts UploadOptions) error {
	logging.FromContext(ctx).Infof("Waiting for code review result")
	logging.FromContext(ctx).Debugf("Initial wait of %s before first poll", opts.InitialWait)

	time.Sleep(opts.InitialWait)

	tried := 0
	for {
		logging.FromContext(ctx).Debugf("Polling for code review result (attempt %d/%d)", tried+1, opts.MaxPolls)

		reviews, err := api.GetBinaryReviewResults(ctx, extensionId, binaryId)
		if err != nil {
			logging.FromContext(ctx).Debugf("Failed to get binary review results during polling: %v", err)
			return err
		}

		logging.FromContext(ctx).Debugf("Current review count: %d, previous count: %d", len(reviews), knownReviews)

		// Review has been updated
		if len(reviews) != knownReviews {
			lastReview := reviews[len(reviews)-1]
			logging.FromContext(ctx).Debugf("Review has been updated, checking status")

			if !lastReview.IsPending() {
				logging.FromContext(ctx).Debugf("Review is no longer pending")
				if lastReview.HasPassed() {
					if lastReview.HasWarnings() {
						logging.FromContext(ctx).Infof("Code review has been passed but with warnings")
						logging.FromContext(ctx).Infof(lastReview.GetSummary())
					} else {
						logging.FromContext(ctx).Infof("Code review has been passed without warnings")
					}

					return nil
				}

				logging.FromContext(ctx).Debugf("Code review failed")
				return fmt.Errorf("code review has not passed: %s", lastReview.GetSummary())
			}

			logging.FromContext(ctx).Debugf("Review is still pending")
		} else {
			logging.FromContext(ctx).Debugf("No new reviews yet, waiting...")
		}

		time.Sleep(opts.PollInterval)
		tried++

		if opts.MaxPolls == tried {
			logging.FromContext(ctx).Infof("Skipping waiting for code review result as it took too long")
			logging.FromContext(ctx).Debugf("Reached maximum retry attempts (%d)", opts.MaxPolls)
			return nil
		}
	}
}
