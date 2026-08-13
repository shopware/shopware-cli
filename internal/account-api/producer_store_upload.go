package account_api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
)

type UploadOptions struct {
	// SkipReviewWait skips waiting for the code review result after the upload.
	SkipReviewWait bool
	// Sleep is used while waiting for the code review result. Defaults to time.Sleep and can be
	// replaced in tests.
	Sleep func(time.Duration)
}

// UploadExtension uploads a new extension version to the Shopware Account, updates the changelog
// and software versions and waits for the automatic code review result.
func UploadExtension(ctx context.Context, producer ProducerAPI, zipExt extension.Extension, zipPath string, opts UploadOptions) error {
	sleep := opts.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	logging.FromContext(ctx).Debugf("Starting upload process for file: %s", zipPath)

	extName, err := zipExt.GetName()
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get extension name: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Extension name: %s", extName)

	ext, err := producer.GetExtensionByName(ctx, extName)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get extension by name '%s': %v", extName, err)
		return err
	}

	logging.FromContext(ctx).Debugf("Found extension with ID: %d", ext.Id)

	binaries, err := producer.GetExtensionBinaries(ctx, ext.Producer.Id, ext.Id)
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

	var foundBinary *ExtensionBinary

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

	availableVersions, err := producer.GetSoftwareVersions(ctx, zipExt.GetType())
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

		create := ExtensionCreate{
			Version:          zipVersion.String(),
			SoftwareVersions: availableVersions.FilterOnVersionStringList(constraint),
			Changelogs: []ExtensionUpdateChangelog{
				{Locale: "de_DE", Text: changelog.German},
				{Locale: "en_GB", Text: changelog.English},
			},
		}

		foundBinary, err = producer.CreateExtensionBinary(ctx, ext.Producer.Id, ext.Id, create)
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

	update := ExtensionUpdate{
		Id:               foundBinary.Id,
		SoftwareVersions: availableVersions.FilterOnVersionStringList(constraint),
		Changelogs: []ExtensionUpdateChangelog{
			{Locale: "de_DE", Text: changelog.German},
			{Locale: "en_GB", Text: changelog.English},
		},
	}

	logging.FromContext(ctx).Debugf("Updating extension binary info for extension ID %d, binary ID %d", ext.Id, foundBinary.Id)

	err = producer.UpdateExtensionBinaryInfo(ctx, ext.Producer.Id, ext.Id, update)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to update extension binary info: %v", err)
		return err
	}

	logging.FromContext(ctx).Infof("Updated changelog. Uploading now the zip to remote")
	logging.FromContext(ctx).Debugf("Uploading zip file from path: %s", zipPath)

	err = producer.UpdateExtensionBinaryFile(ctx, ext.Producer.Id, ext.Id, foundBinary.Id, zipPath)
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

	beforeReviews, err := producer.GetBinaryReviewResults(ctx, ext.Id, foundBinary.Id)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to get binary review results before triggering review: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Found %d existing reviews before triggering new review", len(beforeReviews))

	err = producer.TriggerCodeReview(ctx, ext.Id)
	if err != nil {
		logging.FromContext(ctx).Debugf("Failed to trigger code review: %v", err)
		return err
	}

	logging.FromContext(ctx).Debugf("Successfully triggered code review")

	if opts.SkipReviewWait {
		logging.FromContext(ctx).Debugf("Skipping code review wait as requested by flag")

		return nil
	}

	return waitForCodeReviewResult(ctx, producer, ext.Id, foundBinary.Id, len(beforeReviews), sleep)
}

func waitForCodeReviewResult(ctx context.Context, producer ProducerAPI, extensionId, binaryId, previousReviewCount int, sleep func(time.Duration)) error {
	logging.FromContext(ctx).Infof("Waiting for code review result")
	logging.FromContext(ctx).Debugf("Initial wait of 10 seconds before first poll")

	if err := sleepWithContext(ctx, sleep, 10*time.Second); err != nil {
		return err
	}

	maxTries := 10
	tried := 0
	for {
		logging.FromContext(ctx).Debugf("Polling for code review result (attempt %d/%d)", tried+1, maxTries)

		reviews, err := producer.GetBinaryReviewResults(ctx, extensionId, binaryId)
		if err != nil {
			logging.FromContext(ctx).Debugf("Failed to get binary review results during polling: %v", err)
			return err
		}

		logging.FromContext(ctx).Debugf("Current review count: %d, previous count: %d", len(reviews), previousReviewCount)

		// A new review entry has been added
		if len(reviews) > previousReviewCount {
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

		if err := sleepWithContext(ctx, sleep, 15*time.Second); err != nil {
			return err
		}
		tried++

		if maxTries == tried {
			logging.FromContext(ctx).Infof("Skipping waiting for code review result as it took too long")
			logging.FromContext(ctx).Debugf("Reached maximum retry attempts (%d)", maxTries)
			return nil
		}
	}
}

// sleepWithContext sleeps for the given duration but returns early with the context error when
// the context is cancelled, so callers shut down promptly on CLI cancel/timeout.
func sleepWithContext(ctx context.Context, sleep func(time.Duration), d time.Duration) error {
	done := make(chan struct{})
	go func() {
		sleep(d)
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}
