package project

import (
	"context"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// completeEnvironmentNames completes the persistent -e/--env flag from the
// environments defined in the project configuration. It reuses the same
// lookup as the project commands: FindClosestShopwareProject for the project
// root, SearchConfigPath for the config file location, and ReadConfig (which
// already merges the .local override) for parsing.
func completeEnvironmentNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	ctx := context.Background()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}
	// Completion must stay silent: config warnings would pollute __complete output.
	ctx = logging.DisableLogger(ctx)

	projectRoot, err := shop.FindClosestShopwareProject(true)
	if err != nil || projectRoot == "" {
		projectRoot = "."
	}

	configPath := shop.SearchConfigPath(ctx, projectRoot, projectConfigPath)

	cfg, err := shop.ReadConfig(ctx, configPath, true)
	if err != nil || cfg == nil || len(cfg.Environments) == 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	completions := make([]string, 0, len(cfg.Environments))
	for _, name := range slices.Sorted(maps.Keys(cfg.Environments)) {
		description := ""
		if env := cfg.Environments[name]; env != nil {
			description = env.URL
		}
		if description != "" {
			completions = append(completions, cobra.CompletionWithDesc(name, description))
		} else {
			completions = append(completions, name)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
