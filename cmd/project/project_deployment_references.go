//go:build deployment

package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
)

func deploymentReferenceCompletions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	root, err := resolveDeploymentProjectRoot(nil)
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	backend, err := resolveProjectDeploymentBackend(cmd, root)
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}
	completer, ok := backend.(deployment.ReferenceCompleter)
	if !ok {
		return nil, cobra.ShellCompDirectiveDefault
	}
	references, err := completer.CompleteDeploymentReferences(cmd.Context(), toComplete)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	return references, cobra.ShellCompDirectiveDefault
}
