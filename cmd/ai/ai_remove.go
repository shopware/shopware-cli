package ai

import "github.com/spf13/cobra"

var aiRemoveCmd = &cobra.Command{
	Use:          "remove name",
	Aliases:      []string{"rm"},
	Short:        "Remove an installed Shopware AI skill",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runSkillCommand("remove"),
}

func init() {
	aiRootCmd.AddCommand(aiRemoveCmd)
	addSkillFlags(aiRemoveCmd)
}
