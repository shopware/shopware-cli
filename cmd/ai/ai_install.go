package ai

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// skillsVersion pins the skills.sh CLI the commands run against, so a
// shopware-cli release always drives a known skills.sh behavior.
const skillsVersion = "1.5.18"

// skillsAddArgs builds the `npx skills add ...` argv. shopware-cli is the front
// door: it decides what to install; skills.sh writes the agent configuration.
// `ai add --dry-run` prints this without running it.
func skillsAddArgs(source, skill, agent string, global, assumeYes bool) []string {
	argv := []string{
		"npx", "--yes", "skills@" + skillsVersion, "add", source,
		"--skill", skill,
		"--agent", agent,
	}
	if global {
		argv = append(argv, "--global")
	}
	if assumeYes {
		argv = append(argv, "-y")
	}

	return argv
}

// runSkills runs a skills.sh command via npx. It is a package var so tests can
// substitute it without shelling out.
var runSkills = func(ctx context.Context, argv []string) error {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found: installing skills requires Node.js/npx on PATH", argv[0])
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("skills install failed: %w", err)
	}

	return nil
}
