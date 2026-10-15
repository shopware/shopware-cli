// Package agent detects which AI client invoked the CLI, from the skills.sh
// ecosystem's AI_AGENT environment variable. It never inspects processes.
package agent

import (
	"os"
	"strings"
)

// RecommendedSkillAnnotation is the cobra command annotation key naming the skill
// the CLI recommends to a detected AI client for that command (inherited by
// subcommands). See cmd/ai_hint.go.
const RecommendedSkillAnnotation = "shopware-cli/recommended-skill"

// Detect returns the invoking AI client id (e.g. "claude-code"), or "" when the
// CLI was not invoked by an agent. AI_AGENT is shaped like "<client>_<version>_agent";
// the client id is the first "_"-separated segment, since client ids use hyphens,
// not underscores — the version is never needed.
func Detect() string {
	return strings.Split(strings.TrimSpace(os.Getenv("AI_AGENT")), "_")[0]
}
