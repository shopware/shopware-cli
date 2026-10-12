package ai

import "github.com/spf13/cobra"

var aiAddCmd = &cobra.Command{
	Use:          "add name",
	Short:        "Install a Shopware AI skill",
	Long:         "Install a Shopware AI skill using npx skills. Requires Node.js and npm.\nSkills are installed from their remote repository.",
	Example:      "  shopware-cli ai add shopware-cli --client codex\n  shopware-cli ai add shopware-cli-docker --client claude-code --global",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE:         runSkillCommand("add"),
}

func init() {
	aiRootCmd.AddCommand(aiAddCmd)
	addSkillFlags(aiAddCmd)
}
