package ci

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerminalSections(t *testing.T) {
	for _, color := range []bool{false, true} {
		var output bytes.Buffer
		helper := &terminalHelper{output: &output, color: color}
		section := helper.Section("Preparing release")
		section.End()

		assert.Contains(t, output.String(), "Preparing release finished in")
		assert.Equal(t, color, strings.Contains(output.String(), "\x1b["))
		if color {
			assert.Contains(t, output.String(), "━━ Preparing release")
		} else {
			assert.Contains(t, output.String(), "--- Preparing release ---")
		}
	}
}

func TestOutputHelperPreservesCISections(t *testing.T) {
	for _, tc := range []struct {
		name, environment, start, end string
	}{
		{"GitHub", "GITHUB_ACTIONS", "::group::Preparing release", "::endgroup::"},
		{"GitLab", "GITLAB_CI", "section_start:", "section_end:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", "")
			t.Setenv("GITLAB_CI", "")
			t.Setenv(tc.environment, "true")
			var output bytes.Buffer
			section := New(&output).Section("Preparing release")
			section.End()
			assert.Contains(t, output.String(), tc.start)
			assert.Contains(t, output.String(), tc.end)
		})
	}
}
