package dev

import (
	"fmt"
	"strings"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/tui/prompt"
)

const (
	portConflictID     = "port-conflict"
	portConflictRandom = "random"
	portConflictQuit   = "quit"
)

// newPortConflictPrompt asks whether host ports that are already in use should
// be remapped to random free ports before the containers start. The remapping
// is persisted to the local config override file for future runs.
// portConflictLines formats conflicting ports as "Label: port N" lines.
func portConflictLines(conflicts []dockerpkg.PortConflict) string {
	var b strings.Builder
	for _, conflict := range conflicts {
		fmt.Fprintf(&b, "%s: port %d\n", conflict.Definition.Label, conflict.HostPort)
	}
	return b.String()
}

func newPortConflictPrompt(conflicts []dockerpkg.PortConflict) *prompt.Overlay {
	message := portConflictLines(conflicts) +
		"\nSwitch them to random free ports? The choice is saved to\nthe local config override for future runs."

	return prompt.New(prompt.Options{
		ID:      portConflictID,
		Title:   "Ports already in use",
		Message: message,
		Danger:  true,
		Choices: []prompt.Choice{
			{ID: portConflictRandom, Label: "Use random free ports"},
			{ID: portConflictQuit, Label: "Quit"},
		},
	})
}
