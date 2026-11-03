package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func envOf(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDetectDarkBackgroundSkipsQueryInsideGnuScreen(t *testing.T) {
	queried := false
	dark := detectDarkBackground(envOf(map[string]string{"STY": "1234.pts-0.host"}), func() bool {
		queried = true
		return false
	})

	assert.True(t, dark)
	assert.False(t, queried)
}

func TestDetectDarkBackgroundPrefersColorFgBg(t *testing.T) {
	queried := false
	query := func() bool {
		queried = true
		return true
	}

	assert.False(t, detectDarkBackground(envOf(map[string]string{"COLORFGBG": "0;15", "STY": "x"}), query))
	assert.True(t, detectDarkBackground(envOf(map[string]string{"COLORFGBG": "15;default;0"}), query))
	assert.False(t, queried)
}

func TestDetectDarkBackgroundQueriesTerminalOtherwise(t *testing.T) {
	assert.False(t, detectDarkBackground(envOf(map[string]string{"COLORFGBG": "garbage"}), func() bool { return false }))
	assert.True(t, detectDarkBackground(envOf(nil), func() bool { return true }))
}
