package producer

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
)

func uploadFixture() (*fakeUploadAPI, *fakeExtension) {
	ext := &accountApi.Extension{}
	ext.Id = 7
	ext.Producer.Id = 5

	api := &fakeUploadAPI{
		extension: ext,
		softwareVersions: accountApi.SoftwareVersionList{
			{Name: "6.4.0.0", Selectable: true},
			{Name: "6.5.0.0", Selectable: true},
			{Name: "6.6.0.0", Selectable: false},
		},
	}

	zipExt := &fakeExtension{
		name:       "AcmePlugin",
		version:    "1.0.0",
		constraint: ">=6.5.0",
		extType:    "plugin",
		changelog:  extension.ExtensionChangelog{German: "DE Changelog", English: "EN Changelog"},
	}

	return api, zipExt
}

func skipReviewOptions() UploadOptions {
	return UploadOptions{SkipWaitingForReviewResult: true}
}

func fastPollOptions(maxPolls int) UploadOptions {
	return UploadOptions{
		InitialWait:  0,
		PollInterval: time.Millisecond,
		MaxPolls:     maxPolls,
	}
}

func TestUploadCreatesNewBinary(t *testing.T) {
	api, zipExt := uploadFixture()

	require.NoError(t, Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", skipReviewOptions()))

	require.NotNil(t, api.createdBinary)
	assert.Equal(t, "1.0.0", api.createdBinary.Version)
	assert.Equal(t, []string{"6.5.0.0"}, api.createdBinary.SoftwareVersions)
	assert.Equal(t, []accountApi.ExtensionUpdateChangelog{
		{Locale: "de_DE", Text: "DE Changelog"},
		{Locale: "en_GB", Text: "EN Changelog"},
	}, api.createdBinary.Changelogs)

	require.NotNil(t, api.updatedBinary)
	assert.Equal(t, 1000, api.updatedBinary.Id)
	assert.Equal(t, "/tmp/AcmePlugin.zip", api.uploadedZipPath)
	assert.True(t, api.codeReviewTriggered)
}

func TestUploadUpdatesExistingBinary(t *testing.T) {
	api, zipExt := uploadFixture()
	api.binaries = []*accountApi.ExtensionBinary{
		{Id: 55, Version: "1.0.0"},
		{Id: 56, Version: "2.0.0"},
	}

	require.NoError(t, Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", skipReviewOptions()))

	assert.Nil(t, api.createdBinary)
	require.NotNil(t, api.updatedBinary)
	assert.Equal(t, 55, api.updatedBinary.Id)
}

func TestUploadSkipsAlreadyPublishedBinary(t *testing.T) {
	api, zipExt := uploadFixture()
	api.uploadFileErr = errors.New("the account api returned: BinariesException-40")

	require.NoError(t, Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", skipReviewOptions()))

	assert.False(t, api.codeReviewTriggered)
}

func TestUploadFailsOnUploadError(t *testing.T) {
	api, zipExt := uploadFixture()
	api.uploadFileErr = errors.New("boom")

	err := Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", skipReviewOptions())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "upload extension binary file")
	assert.False(t, api.codeReviewTriggered)
}

func TestUploadWaitsForPassedCodeReview(t *testing.T) {
	api, zipExt := uploadFixture()

	var passed accountApi.BinaryReviewResult
	passed.Type.Id = 3

	api.reviewResults = [][]accountApi.BinaryReviewResult{
		{},       // before triggering the review
		{passed}, // first poll
	}

	require.NoError(t, Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", fastPollOptions(3)))

	assert.True(t, api.codeReviewTriggered)
	assert.GreaterOrEqual(t, api.reviewResultCalls, 2)
}

func TestUploadFailsWhenCodeReviewFails(t *testing.T) {
	api, zipExt := uploadFixture()

	var failed accountApi.BinaryReviewResult
	failed.Type.Id = 2

	api.reviewResults = [][]accountApi.BinaryReviewResult{
		{},
		{failed},
	}

	err := Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", fastPollOptions(3))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "code review has not passed")
}

func TestUploadGivesUpWaitingAfterMaxPolls(t *testing.T) {
	api, zipExt := uploadFixture()

	var pending accountApi.BinaryReviewResult
	pending.Type.Id = 4

	api.reviewResults = [][]accountApi.BinaryReviewResult{
		{},
		{pending},
	}

	require.NoError(t, Upload(t.Context(), api, zipExt, "/tmp/AcmePlugin.zip", fastPollOptions(2)))

	// one call before triggering the review + MaxPolls polls
	assert.Equal(t, 3, api.reviewResultCalls)
}
