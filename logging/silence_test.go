package logging

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSilentError(t *testing.T) {
	sentinel := NewSilent("already reported")
	wrapped := fmt.Errorf("cannot complete command: %w", sentinel)

	var silent SilenceError
	assert.ErrorAs(t, wrapped, &silent)
	assert.ErrorIs(t, wrapped, sentinel)
	assert.False(t, errors.Is(wrapped, NewSilent("already reported")))
}
