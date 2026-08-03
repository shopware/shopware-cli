package account_api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
