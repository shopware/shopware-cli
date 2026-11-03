package tracking

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCancelledError(t *testing.T) {
	sentinel := NewCancelled("creation cancelled")
	wrapped := fmt.Errorf("cannot create project: %w", sentinel)

	var cancelled Cancelled
	assert.ErrorAs(t, wrapped, &cancelled)
	assert.ErrorIs(t, wrapped, sentinel)
	assert.False(t, errors.Is(wrapped, NewCancelled("creation cancelled")))
}
