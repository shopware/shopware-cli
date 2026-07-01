package devtui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func set(names ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

func TestAllRunning(t *testing.T) {
	t.Run("all defined services running", func(t *testing.T) {
		assert.True(t, allRunning(set("web", "database"), set("web", "database", "adminer")))
	})

	t.Run("a newly added service is not running", func(t *testing.T) {
		// worker was just added to compose.yaml but is not up yet.
		assert.False(t, allRunning(set("web", "database", "worker"), set("web", "database")))
	})

	t.Run("empty defined imposes no constraint", func(t *testing.T) {
		assert.True(t, allRunning(nil, set("web")))
	})
}
