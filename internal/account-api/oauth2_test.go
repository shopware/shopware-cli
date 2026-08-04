package account_api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRandomState(t *testing.T) {
	state, err := generateRandomState()
	require.NoError(t, err)
	assert.Len(t, state, 32) // 16 bytes hex-encoded

	other, err := generateRandomState()
	require.NoError(t, err)
	assert.NotEqual(t, state, other)
}
