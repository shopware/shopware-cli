package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetect(t *testing.T) {
	cases := map[string]string{
		"claude-code_2-1-289_agent": "claude-code", // the real Claude Code value
		"cursor_1.2.3_agent":        "cursor",      // dotted version
		"codex_12_agent":            "codex",       // single-number version
		"codex":                     "codex",       // bare client, no suffix
		"":                          "",
		"   ":                       "",
	}

	for env, want := range cases {
		t.Setenv("AI_AGENT", env)
		assert.Equal(t, want, Detect(), "AI_AGENT=%q", env)
	}
}
