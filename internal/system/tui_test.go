package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTUIActive(t *testing.T) {
	t.Cleanup(func() { SetTUIActive(false) })

	assert.False(t, IsTUIActive())

	SetTUIActive(true)
	assert.True(t, IsTUIActive())

	SetTUIActive(false)
	assert.False(t, IsTUIActive())
}
