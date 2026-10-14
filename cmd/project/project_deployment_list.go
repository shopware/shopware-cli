//go:build deployment

package project

import (
	"fmt"
	"slices"

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
	rollouts, listErr := history.ListRollouts(cmd.Context())
	if listErr != nil && len(rollouts) == 0 {
		return fmt.Errorf("list deployments: %w", listErr)
	}

	columns := []tui.TableColumn{
		{Title: "Deployment", JSONKey: "deployment"},
		{Title: "Status", JSONKey: "active"},
		{Title: "Deployed at (UTC)", JSONKey: "deployed_at"},
	}
	showHost := slices.ContainsFunc(rollouts, func(rollout deployment.Rollout) bool { return rollout.Host != "" })
	if showHost {
		columns = append([]tui.TableColumn{{Title: "Host", JSONKey: "host"}}, columns...)
	}
	table := tui.NewTable(columns...)
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
		row := map[string]any{
			"release":     rollout.Reference,
			"deployment":  rollout.Deployment.Reference,
			"active":      rollout.Active,
			"deployed_at": rollout.DeployedAt,
		}
		cells := []any{rollout.Deployment.DisplayName(), status, deployedAt}
		if showHost {
			row["host"] = rollout.Host
			cells = append([]any{rollout.Host}, cells...)
		}
		table.AddRowWithJSON(row, cells...)
	}
	if err := table.Write(cmd.OutOrStdout(), format); err != nil {
		return err
	}
	if listErr != nil {
		return fmt.Errorf("list deployments: %w", listErr)
	}
	return nil
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentListCmd)
	projectDeploymentListCmd.Flags().String("format", string(tui.TableFormatTable), "Output format (table or json)")
}
