package symfony

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeValueRecursesMaps(t *testing.T) {
	dst := map[string]any{
		"framework": map[string]any{
			"secret": "base",
			"cache":  map[string]any{"app": "filesystem"},
		},
	}
	src := map[string]any{
		"framework": map[string]any{
			"cache":    map[string]any{"app": "redis"},
			"profiler": map[string]any{"enabled": true},
		},
	}

	merged := mergeValue(dst, src).(map[string]any)
	framework := merged["framework"].(map[string]any)

	assert.Equal(t, "base", framework["secret"], "untouched keys survive")
	assert.Equal(t, "redis", framework["cache"].(map[string]any)["app"], "scalar overridden")
	assert.Equal(t, true, framework["profiler"].(map[string]any)["enabled"], "new key added")
}

func TestMergeValueReplacesSequences(t *testing.T) {
	dst := map[string]any{"channels": []any{"main", "event"}}
	src := map[string]any{"channels": []any{"deprecation"}}

	merged := mergeValue(dst, src).(map[string]any)

	// Symfony replaces lists rather than concatenating them.
	assert.Equal(t, []any{"deprecation"}, merged["channels"])
}

func TestMergeValueDoesNotMutateInputs(t *testing.T) {
	dst := map[string]any{"a": map[string]any{"x": 1}}
	src := map[string]any{"a": map[string]any{"y": 2}}

	_ = mergeValue(dst, src)

	// dst's nested map must be untouched so lower-precedence callers keep their
	// original data.
	assert.Equal(t, map[string]any{"x": 1}, dst["a"])
}

func TestMergeValueScalarReplacesMap(t *testing.T) {
	dst := map[string]any{"handler_id": map[string]any{"nested": true}}
	src := map[string]any{"handler_id": nil}

	merged := mergeValue(dst, src).(map[string]any)
	assert.Nil(t, merged["handler_id"])
}
