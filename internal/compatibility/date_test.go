package compatibility

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestValidateDate(t *testing.T) {
	assert.NoError(t, ValidateDate(""))
	assert.NoError(t, ValidateDate("2026-02-11"))
	assert.Error(t, ValidateDate("11-02-2026"))
}

func TestIsAtLeast(t *testing.T) {
	result, err := IsAtLeast("", "2026-01-01")
	assert.NoError(t, err)
	assert.True(t, result)

	result, err = IsAtLeast("2026-02-11", "2026-02-11")
	assert.NoError(t, err)
	assert.True(t, result)

	result, err = IsAtLeast("2026-02-11", "2026-03-01")
	assert.NoError(t, err)
	assert.False(t, result)

	_, err = IsAtLeast("invalid", "2026-03-01")
	assert.Error(t, err)

	_, err = IsAtLeast("2026-02-11", "invalid")
	assert.Error(t, err)
}

func TestTodayDate(t *testing.T) {
	now = func() time.Time {
		return time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
	}
	t.Cleanup(func() {
		now = time.Now
	})

	assert.Equal(t, "2026-02-11", TodayDate())
}

func TestDefaultDate(t *testing.T) {
	assert.Equal(t, "2026-02-11", DefaultDate())
}
