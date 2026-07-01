package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckboxRow(t *testing.T) {
	row := CheckboxRow("[x]", "Queue worker", "running", SuccessColor, true)

	// The glyph, label and status all render into a single KVRow line.
	assert.Contains(t, row, "[x]")
	assert.Contains(t, row, "Queue worker")
	assert.Contains(t, row, "running")
	assert.True(t, strings.HasSuffix(row, "\n"))
}
