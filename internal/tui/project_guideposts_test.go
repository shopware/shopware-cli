package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestProjectWorkflowGuideposts(t *testing.T) {
	for _, template := range []string{"", ".github/workflows/ci.yml", ".gitlab-ci.yml"} {
		t.Run("template="+template, func(t *testing.T) {
			output := ansi.Strip(ProjectWorkflowGuideposts(template))
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
