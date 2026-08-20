package update

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckHandleWaitsForCompletion(t *testing.T) {
	handle := NewCheckHandle()
	wantErr := errors.New("check failed")
	want := CheckResult{Err: wantErr}

	go handle.Complete(want)

	assert.Equal(t, want, handle.Wait(t.Context()))
	assert.Equal(t, want, handle.Wait(t.Context()))
}

func TestCheckHandleWaitHonorsContextCancellation(t *testing.T) {
	handle := NewCheckHandle()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()

	result := handle.Wait(ctx)

	assert.ErrorIs(t, result.Err, context.DeadlineExceeded)
	assert.Nil(t, result.Release)
}

func TestCheckHandleCompleteOnlyUsesFirstResult(t *testing.T) {
	handle := NewCheckHandle()
	first := CheckResult{Release: &ReleaseInfo{Version: "v1.0.0"}}
	second := CheckResult{Release: &ReleaseInfo{Version: "v2.0.0"}}

	handle.Complete(first)
	handle.Complete(second)

	result := handle.Wait(t.Context())
	require.NotNil(t, result.Release)
	assert.Equal(t, "v1.0.0", result.Release.Version)
}
