package system

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckIncompatibilities(t *testing.T) {
	t.Run("no incompatibilities on non-darwin", func(t *testing.T) {
		t.Setenv("HOME", "/tmp/test-home")
		incompatibilities := CheckIncompatibilities(false, "/tmp/project")
		assert.Empty(t, incompatibilities)
	})
}

func TestRenderMissingDependencies(t *testing.T) {
	t.Run("docker not running shows start message", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker", Reason: "not running"},
		})
		assert.Contains(t, out, "Start Docker and try again.")
		assert.NotContains(t, out, "install one of")
	})

	t.Run("docker not installed shows install message", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker", Reason: "not installed"},
		})
		assert.Contains(t, out, "Install Docker and try again.")
		assert.NotContains(t, out, "install one of")
	})

	t.Run("missing php and composer shows install links", func(t *testing.T) {
		out := RenderMissingDependencies(false, []MissingDependency{
			{Name: "PHP 8.2+", Reason: "not installed"},
			{Name: "Composer", Reason: "not installed"},
		})
		assert.Contains(t, out, "install one of")
		assert.Contains(t, out, "https://www.php.net/downloads.php")
		assert.Contains(t, out, "https://getcomposer.org/")
	})
}
