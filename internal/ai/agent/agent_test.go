package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParse(t *testing.T) {
	cases := map[string]string{
		"claude-code_2-1-289_agent": "claude-code", // the real Claude Code value
		"cursor_1.2.3_agent":        "cursor",      // dotted version
		"codex_12_agent":            "codex",       // single-number version
		"codex":                     "codex",       // bare client, no suffix
		"foo_bar_1-2_agent":         "foo_bar",     // client id with an underscore
		"":                          "",
		"   ":                       "",
	}

	for in, want := range cases {
		assert.Equal(t, want, parse(in), "parse(%q)", in)
	}
}

func TestDetectReadsEnv(t *testing.T) {
	t.Setenv("AI_AGENT", "claude-code_2-1-289_agent")
	assert.Equal(t, "claude-code", Detect())

	t.Setenv("AI_AGENT", "")
	assert.Equal(t, "", Detect())
}
