package tracking

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFailedAt(t *testing.T) {
	assert.NoError(t, FailedAt("scaffold", nil))

	cause := errors.New("disk full")
	wrapped := fmt.Errorf("cannot create project: %w", FailedAt("scaffold", cause))

	var failed *FailedStepError
	assert.ErrorAs(t, wrapped, &failed)
	assert.Equal(t, "scaffold", failed.Step)
	assert.ErrorIs(t, wrapped, cause)
	assert.EqualError(t, wrapped, "cannot create project: disk full")
}
