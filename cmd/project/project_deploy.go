package project

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
)

var (
	projectDeployList     bool
	projectDeployRollback string
)

var projectDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the project to a configured environment",
	Long: "Deploy the project to an environment that exposes a Deployer (currently the ssh environment type). " +
		"Use --list to inspect releases on the target, --rollback to roll back to the previous release, " +
		"or --rollback=<name> to roll back to a specific release.",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		exec, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		deployer := exec.Deployer()
		if deployer == nil {
			return fmt.Errorf("environment %q (type %s) does not support deploy; use an ssh environment", environmentName, exec.Type())
		}

		ctx := cmd.Context()

		if projectDeployList {
			releases, err := deployer.ListReleases(ctx)
			if err != nil {
				return err
			}
			return printReleases(cmd, releases)
		}

		if cmd.Flags().Changed("rollback") {
			return deployer.Rollback(ctx, projectDeployRollback)
		}

		return deployer.Deploy(ctx)
	},
}

func printReleases(cmd *cobra.Command, releases []executor.Release) error {
	if len(releases) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No releases found on target.")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "RELEASE\tCREATED\tCURRENT")
	for _, r := range releases {
		created := ""
		if !r.CreatedAt.IsZero() {
			created = r.CreatedAt.Format("2006-01-02 15:04:05")
		}
		marker := ""
		if r.Current {
			marker = "*"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, created, marker)
	}
	return w.Flush()
}

func init() {
	projectDeployCmd.Flags().BoolVar(&projectDeployList, "list", false, "List releases known on the target environment")
	projectDeployCmd.Flags().StringVar(&projectDeployRollback, "rollback", "", "Roll back to the previous release, or to a specific release when a value is provided")
	projectDeployCmd.Flag("rollback").NoOptDefVal = ""
	projectRootCmd.AddCommand(projectDeployCmd)
}
