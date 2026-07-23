package devtui

import (
	"github.com/shopware/shopware-cli/internal/tui/prompt"
)

const (
	stopConfirmID     = "stop-confirm"
	stopConfirmStop   = "stop"
	stopConfirmQuit   = "quit"
	stopConfirmCancel = "cancel"
)

// newStopConfirm asks whether leaving the dashboard should also stop the
// running Docker containers.
func newStopConfirm() *prompt.Overlay {
	return prompt.New(prompt.Options{
		ID:      stopConfirmID,
		Title:   "Leaving the workspace",
		Message: "Do you also want to stop the running Docker containers?\nEither way you can restart them anytime with shopware-cli project dev.",
		Danger:  true,
		Choices: []prompt.Choice{
			{ID: stopConfirmStop, Label: "Stop containers & quit"},
			{ID: stopConfirmQuit, Label: "Quit, keep running"},
			{ID: stopConfirmCancel, Label: "Cancel"},
		},
	})
}
