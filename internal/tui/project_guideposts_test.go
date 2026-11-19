package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestProjectWorkflowGuideposts(t *testing.T) {
	for _, template := range []string{"", ".github/workflows/ci.yml", ".gitlab-ci.yml"} {
		t.Run("template="+template, func(t *testing.T) {
			output := ansi.Strip(ProjectWorkflowGuideposts(template, false, true))
			for _, scope := range []string{"project", "extension"} {
				path := "."
				if scope == "extension" {
					path = "custom/plugins/MyPlugin"
				}
				for _, command := range []string{"validate", "format", "fix"} {
					expected := "shopware-cli " + scope + " " + command + " " + path
					if command == "fix" {
						expected += " --dry-run"
					}
					assert.Contains(t, output, expected)
				}
			}
			assert.Contains(t, output, "shopware-cli project dump --output dump.sql")
			assert.Contains(t, output, "shopware-cli project dump --help")
			assert.NotContains(t, output, "project deploy")
			assert.NotContains(t, output, "admin / shopware")
			if template == "" {
				assert.NotContains(t, output, ".github/workflows")
				assert.NotContains(t, output, ".gitlab-ci.yml")
			} else {
				assert.Contains(t, output, template)
				other := ".github/workflows/ci.yml"
				if template == other {
					other = ".gitlab-ci.yml"
				}
				assert.NotContains(t, output, other)
			}
		})
	}
}

func TestProjectWorkflowGuidepostsDockerPrerequisites(t *testing.T) {
	for _, docker := range []bool{false, true} {
		output := ansi.Strip(ProjectWorkflowGuideposts("", docker, true))
		assert.Equal(t, docker, strings.Contains(output, "host PHP 8.2+, Composer and Node.js 20+ (npm)"))
		assert.Contains(t, output, "shopware-cli project dump --output dump.sql")
		assert.NotContains(t, output, "docker run")
	}
}

// Plain output must not resolve adaptive colours: those can read stdin and write
// terminal queries to stdout, even when the hints themselves go to stderr.
func TestProjectWorkflowGuidepostsPlainDoesNotQueryTerminal(t *testing.T) {
	original := hasDarkBackground
	hasDarkBackground = func() bool {
		t.Fatal("plain guideposts queried the terminal")
		return false
	}
	t.Cleanup(func() { hasDarkBackground = original })
	output := ProjectWorkflowGuideposts(".github/workflows/ci.yml", true, false)
	assert.NotContains(t, output, "\x1b")
	assert.Contains(t, output, "shopware-cli project validate .")
	assert.Contains(t, output, ".github/workflows/ci.yml")
}
