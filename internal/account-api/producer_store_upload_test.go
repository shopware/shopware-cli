package account_api

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

func noopSleep(_ time.Duration) {}

func reviewWithType(typeId int) BinaryReviewResult {
	var review BinaryReviewResult
	review.Type.Id = typeId
	return review
}

func TestWaitForCodeReviewResultPassed(t *testing.T) {
	// Type id 3 means the automatic code review succeeded
	producer := &fakeProducer{
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			return []BinaryReviewResult{reviewWithType(1), reviewWithType(3)}, nil
		},
	}

	err := waitForCodeReviewResult(t.Context(), producer, 1, 1, 1, noopSleep)

	require.NoError(t, err)
}

func TestWaitForCodeReviewResultFailed(t *testing.T) {
	producer := &fakeProducer{
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			review := reviewWithType(2)
			review.SubCheckResults = append(review.SubCheckResults, struct {
				SubCheck    string `json:"subCheck"`
				Status      string `json:"status"`
				Passed      bool   `json:"passed"`
				Message     string `json:"message"`
				HasWarnings bool   `json:"hasWarnings"`
			}{SubCheck: "php", Passed: false, Message: "broken"})

			return []BinaryReviewResult{reviewWithType(1), review}, nil
		},
	}

	err := waitForCodeReviewResult(t.Context(), producer, 1, 1, 1, noopSleep)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "code review has not passed")
	assert.Contains(t, err.Error(), "broken")
}

func TestWaitForCodeReviewResultPollsUntilReviewAppears(t *testing.T) {
	calls := 0
	producer := &fakeProducer{
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			calls++
			if calls < 3 {
				// no new review yet
				return []BinaryReviewResult{reviewWithType(1)}, nil
			}
			return []BinaryReviewResult{reviewWithType(1), reviewWithType(3)}, nil
		},
	}

	err := waitForCodeReviewResult(t.Context(), producer, 1, 1, 1, noopSleep)

	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestWaitForCodeReviewResultGivesUpAfterMaxTries(t *testing.T) {
	calls := 0
	producer := &fakeProducer{
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			calls++
			return []BinaryReviewResult{reviewWithType(1)}, nil
		},
	}

	err := waitForCodeReviewResult(t.Context(), producer, 1, 1, 1, noopSleep)

	require.NoError(t, err)
	assert.Equal(t, 10, calls)
}

func TestWaitForCodeReviewResultShrinkingListDoesNotPanic(t *testing.T) {
	producer := &fakeProducer{
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			// transient glitch: fewer reviews than before
			return nil, nil
		},
	}

	err := waitForCodeReviewResult(t.Context(), producer, 1, 1, 3, noopSleep)

	require.NoError(t, err)
}

func TestWaitForCodeReviewResultContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := waitForCodeReviewResult(ctx, &fakeProducer{}, 1, 1, 0, noopSleep)

	require.ErrorIs(t, err, context.Canceled)
}

func TestSleepWithContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := sleepWithContext(ctx, time.Sleep, time.Hour)

	require.ErrorIs(t, err, context.Canceled)
}

func TestSleepWithContextCompletes(t *testing.T) {
	require.NoError(t, sleepWithContext(t.Context(), func(time.Duration) {}, time.Hour))
}

func TestUploadExtensionCreatesBinary(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "ext.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o644))

	var created ExtensionCreate
	var updated ExtensionUpdate
	var uploadedZip string
	var triggered bool

	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, name string) (*Extension, error) {
			assert.Equal(t, "TestPlugin", name)
			var ext Extension
			ext.Id = 5
			ext.Producer.Id = 1
			return &ext, nil
		},
		getExtensionBinariesFn: func(_ context.Context, producerId, extensionId int) ([]*ExtensionBinary, error) {
			assert.Equal(t, 1, producerId)
			assert.Equal(t, 5, extensionId)
			return nil, nil
		},
		getSoftwareVersionsFn: func(_ context.Context, generation string) (*SoftwareVersionList, error) {
			assert.Equal(t, extension.TypePlatformPlugin, generation)
			return &SoftwareVersionList{
				{Name: "6.5.0.0", Selectable: true},
				{Name: "6.4.0.0", Selectable: true},
			}, nil
		},
		createExtensionBinaryFn: func(_ context.Context, _, _ int, create ExtensionCreate) (*ExtensionBinary, error) {
			created = create
			return &ExtensionBinary{Id: 9, Version: create.Version}, nil
		},
		updateExtensionBinaryInfoFn: func(_ context.Context, _, _ int, update ExtensionUpdate) error {
			updated = update
			return nil
		},
		updateExtensionBinaryFileFn: func(_ context.Context, _, _, _ int, path string) error {
			uploadedZip = path
			return nil
		},
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			return nil, nil
		},
		triggerCodeReviewFn: func(_ context.Context, extensionId int) error {
			assert.Equal(t, 5, extensionId)
			triggered = true
			return nil
		},
	}

	ver, err := version.NewVersion("1.2.3")
	require.NoError(t, err)
	constraint, err := version.NewConstraint(">=6.5.0")
	require.NoError(t, err)

	zipExt := &fakeExtension{
		name:       "TestPlugin",
		path:       t.TempDir(),
		extVersion: ver,
		constraint: &constraint,
		changelog:  &extension.ExtensionChangelog{German: "de notes", English: "en notes"},
	}

	require.NoError(t, UploadExtension(t.Context(), producer, zipExt, zipPath, UploadOptions{
		SkipReviewWait: true,
		Sleep:          noopSleep,
	}))

	assert.Equal(t, "1.2.3", created.Version)
	assert.Equal(t, []string{"6.5.0.0"}, created.SoftwareVersions)
	assert.Equal(t, 9, updated.Id)
	assert.Equal(t, zipPath, uploadedZip)
	assert.True(t, triggered)
}

func TestUploadExtensionUpdatesExistingBinary(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "ext.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o644))

	var created bool
	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, _ string) (*Extension, error) {
			var ext Extension
			ext.Id = 5
			ext.Producer.Id = 1
			return &ext, nil
		},
		getExtensionBinariesFn: func(_ context.Context, _, _ int) ([]*ExtensionBinary, error) {
			return []*ExtensionBinary{{Id: 3, Version: "1.0.0"}}, nil
		},
		getSoftwareVersionsFn: func(_ context.Context, _ string) (*SoftwareVersionList, error) {
			return &SoftwareVersionList{{Name: "6.5.0.0", Selectable: true}}, nil
		},
		createExtensionBinaryFn: func(_ context.Context, _, _ int, _ ExtensionCreate) (*ExtensionBinary, error) {
			created = true
			return &ExtensionBinary{}, nil
		},
		updateExtensionBinaryInfoFn: func(_ context.Context, _, _ int, update ExtensionUpdate) error {
			assert.Equal(t, 3, update.Id)
			return nil
		},
		updateExtensionBinaryFileFn: func(_ context.Context, _, _, binaryId int, _ string) error {
			assert.Equal(t, 3, binaryId)
			return nil
		},
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			return []BinaryReviewResult{reviewWithType(3)}, nil
		},
		triggerCodeReviewFn: func(_ context.Context, _ int) error { return nil },
	}

	ver, err := version.NewVersion("1.0.0")
	require.NoError(t, err)
	constraint, err := version.NewConstraint(">=6.5.0")
	require.NoError(t, err)

	require.NoError(t, UploadExtension(t.Context(), producer, &fakeExtension{
		extVersion: ver,
		constraint: &constraint,
	}, zipPath, UploadOptions{SkipReviewWait: true, Sleep: noopSleep}))

	assert.False(t, created)
}

func TestUploadExtensionWaitsForReview(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "ext.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o644))

	reviewCalls := 0
	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, _ string) (*Extension, error) {
			var ext Extension
			ext.Id = 5
			ext.Producer.Id = 1
			return &ext, nil
		},
		getExtensionBinariesFn: func(_ context.Context, _, _ int) ([]*ExtensionBinary, error) {
			return []*ExtensionBinary{{Id: 3, Version: "1.0.0"}}, nil
		},
		getSoftwareVersionsFn: func(_ context.Context, _ string) (*SoftwareVersionList, error) {
			return &SoftwareVersionList{{Name: "6.5.0.0", Selectable: true}}, nil
		},
		updateExtensionBinaryInfoFn: func(_ context.Context, _, _ int, _ ExtensionUpdate) error { return nil },
		updateExtensionBinaryFileFn: func(_ context.Context, _, _, _ int, _ string) error { return nil },
		getBinaryReviewResultsFn: func(_ context.Context, _, _ int) ([]BinaryReviewResult, error) {
			reviewCalls++
			if reviewCalls == 1 {
				// before trigger
				return nil, nil
			}
			return []BinaryReviewResult{reviewWithType(3)}, nil
		},
		triggerCodeReviewFn: func(_ context.Context, _ int) error { return nil },
	}

	ver, err := version.NewVersion("1.0.0")
	require.NoError(t, err)
	constraint, err := version.NewConstraint(">=6.5.0")
	require.NoError(t, err)

	require.NoError(t, UploadExtension(t.Context(), producer, &fakeExtension{
		extVersion: ver,
		constraint: &constraint,
	}, zipPath, UploadOptions{Sleep: noopSleep}))

	assert.GreaterOrEqual(t, reviewCalls, 2)
}

func TestUploadExtensionSkipsPublishedBinary(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "ext.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("zip"), 0o644))

	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, _ string) (*Extension, error) {
			var ext Extension
			ext.Id = 5
			ext.Producer.Id = 1
			return &ext, nil
		},
		getExtensionBinariesFn: func(_ context.Context, _, _ int) ([]*ExtensionBinary, error) {
			return []*ExtensionBinary{{Id: 3, Version: "1.0.0"}}, nil
		},
		getSoftwareVersionsFn: func(_ context.Context, _ string) (*SoftwareVersionList, error) {
			return &SoftwareVersionList{{Name: "6.5.0.0", Selectable: true}}, nil
		},
		updateExtensionBinaryInfoFn: func(_ context.Context, _, _ int, _ ExtensionUpdate) error {
			return nil
		},
		updateExtensionBinaryFileFn: func(_ context.Context, _, _, _ int, _ string) error {
			return errors.New("BinariesException-40: already published")
		},
	}

	ver, err := version.NewVersion("1.0.0")
	require.NoError(t, err)
	constraint, err := version.NewConstraint(">=6.5.0")
	require.NoError(t, err)

	require.NoError(t, UploadExtension(t.Context(), producer, &fakeExtension{
		extVersion: ver,
		constraint: &constraint,
	}, zipPath, UploadOptions{Sleep: noopSleep}))
}
