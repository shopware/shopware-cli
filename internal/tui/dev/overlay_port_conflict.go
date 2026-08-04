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
func newPortConflictPrompt(conflicts []dockerpkg.PortConflict) *prompt.Overlay {
	var message strings.Builder
	for _, conflict := range conflicts {
		fmt.Fprintf(&message, "%s: port %d\n", conflict.Definition.Label, conflict.HostPort)
	}
	message.WriteString("\nSwitch them to random free ports? The choice is saved to\nthe local config override for future runs.")

	return prompt.New(prompt.Options{
		ID:      portConflictID,
		Title:   "Ports already in use",
		Message: message.String(),
		Danger:  true,
		Choices: []prompt.Choice{
			{ID: portConflictRandom, Label: "Use random free ports"},
			{ID: portConflictQuit, Label: "Quit"},
		},
	})
}
