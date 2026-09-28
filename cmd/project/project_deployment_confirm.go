//go:build deployment

package project

import (
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/system"
)

func confirmDeployment(cmd *cobra.Command, artifact deployment.Deployment, kind deploymentSelectionKind) (bool, error) {
	if err := cmd.Context().Err(); err != nil {
		return false, err
	}
	// The root command sets this from stdin's TTY state and --no-interaction.
	if !system.IsInteractionEnabled(cmd.Context()) {
		return true, nil
	}
	confirmed := false
	err := deploymentConfirmationForm(cmd, artifact, kind, &confirmed).RunWithContext(cmd.Context())
	if cmd.Context().Err() != nil {
		return false, cmd.Context().Err()
	}
	if errors.Is(err, huh.ErrUserAborted) {
		return false, nil
	}
	return confirmed && err == nil, err
}

func deploymentConfirmationForm(cmd *cobra.Command, artifact deployment.Deployment, kind deploymentSelectionKind, confirmed *bool) *huh.Form {
	environment, _ := cmd.Flags().GetString("env")
	if environment == "" {
		environment = "local"
	}
	action := "Roll out"
	if kind == rollbackSelection {
		action = "Reactivate"
	}
	return huh.NewForm(huh.NewGroup(
		huh.NewSelect[bool]().
			Title(fmt.Sprintf("%s deployment %q on environment %q?", action, artifact.DisplayName(), environment)).
			Options(huh.NewOption("Yes", true), huh.NewOption("No", false)).
			Value(confirmed),
	)).WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr())
}
