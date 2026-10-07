// Package agent detects which AI client invoked the CLI, from the skills.sh
// ecosystem's AI_AGENT environment variable. It never inspects processes.
package agent

import (
	"os"
	"regexp"
	"strings"
)

// versionSegment matches a trailing version like "2-1-289" or "1.2.3".
var versionSegment = regexp.MustCompile(`^[0-9]+([.\-][0-9]+)*$`)

// Detect returns the invoking AI client id (e.g. "claude-code"), or "" when the
// CLI was not invoked by an agent. The id is the one skills.sh expects as
// --agent, read from AI_AGENT (shaped like "<client>_<version>_agent").
func Detect() string {
	return parse(os.Getenv("AI_AGENT"))
}

// parse extracts the client id from an AI_AGENT value by dropping a trailing
// "_agent" marker and "_<version>" segment. A non-empty value is always treated
// as an agent; anything left over is the client id.
func parse(v string) string {
	parts := strings.Split(strings.TrimSpace(v), "_")

	if len(parts) > 1 && parts[len(parts)-1] == "agent" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > 1 && versionSegment.MatchString(parts[len(parts)-1]) {
		parts = parts[:len(parts)-1]
	}

	return strings.Join(parts, "_")
}
