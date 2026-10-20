//go:build deployment

package project

import (
	"strings"

	"github.com/spf13/cobra"
)

func deploymentReferenceCompletions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeDeploymentSelection(cmd, args, toComplete, rolloutSelection)
}

func rollbackReferenceCompletions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeDeploymentSelection(cmd, args, toComplete, rollbackSelection)
}

func completeDeploymentSelection(cmd *cobra.Command, args []string, prefix string, kind deploymentSelectionKind) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	fallback := cobra.ShellCompDirectiveDefault
	if kind == rollbackSelection {
		fallback = cobra.ShellCompDirectiveNoFileComp
	}
	root, err := resolveDeploymentProjectRoot(nil)
	if err != nil {
		return nil, fallback
	}
	backend, err := resolveProjectDeploymentBackend(cmd, root)
	if err != nil {
		return nil, fallback
	}
	candidates, err := deploymentSelectionCandidates(cmd.Context(), backend, kind)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	references := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate.Deployment.Reference, prefix) {
			references = append(references, candidate.Deployment.Reference)
		}
	}
	return references, fallback
}
