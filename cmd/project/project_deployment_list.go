//go:build deployment

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/tui"
)

var projectDeploymentListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List successful deployment rollouts",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		root, err := resolveDeploymentProjectRoot(nil)
		if err != nil {
			return err
		}
		target, err := resolveProjectDeploymentBackend(cmd, root)
		if err != nil {
			return err
		}
		formatName, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		format, err := tui.ParseTableFormat(formatName)
		if err != nil {
			return err
		}
		return runProjectDeploymentList(cmd, target, format)
	},
}

func runProjectDeploymentList(cmd *cobra.Command, target deployment.Backend, format tui.TableFormat) error {
	history, ok := target.(deployment.RolloutHistory)
	if !ok {
		return fmt.Errorf("listing deployments with backend %q: %w", target.Type(), deployment.ErrNotSupported)
	}
	rollouts, err := history.ListRollouts(cmd.Context())
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}

	table := tui.NewTable(
		tui.TableColumn{Title: "Deployment", JSONKey: "deployment"},
		tui.TableColumn{Title: "Status", JSONKey: "active"},
		tui.TableColumn{Title: "Deployed at (UTC)", JSONKey: "deployed_at"},
	)
	for _, rollout := range rollouts {
		status := "Inactive"
		if rollout.Active {
			status = "Active"
		}
		deployedAt := "—"
		if rollout.DeployedAt != nil {
			deployedAt = rollout.DeployedAt.UTC().Format("2006-01-02 15:04:05")
		}
		// Keep the activation identity in structured output, not the default table.
		table.AddRowWithJSON(map[string]any{
			"release":     rollout.Reference,
			"deployment":  rollout.Deployment.Reference,
			"active":      rollout.Active,
			"deployed_at": rollout.DeployedAt,
		}, rollout.Deployment.DisplayName(), status, deployedAt)
	}
	return table.Write(cmd.OutOrStdout(), format)
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentListCmd)
	projectDeploymentListCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table or json)")
}
