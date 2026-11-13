package project

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
)

var projectScheduledTaskRunnerCmd = &cobra.Command{
	Use:   "scheduled-task-runner",
	Short: "Run Shopware scheduled tasks every minute",
	Long: `Spawn a short-lived scheduled task run every minute, delayed by a random
jitter of up to 30 seconds.

Each run executes a single scheduled task cycle and exits, so no long-running
PHP process lingers. When a run takes longer than a minute, the next one is
started anyway, but never more than two runs are active at the same time. A run
that takes longer than ten minutes is stopped.`,
	Args: cobra.NoArgs,
	RunE: func(cobraCmd *cobra.Command, _ []string) error {
		isVerbose, _ := cobraCmd.Flags().GetBool("verbose")
		gracefulStopLimit, _ := cobraCmd.Flags().GetUint("graceful-stop-limit")

		projectRoot, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cobraCmd, projectRoot)
		if err != nil {
			return err
		}

		runArgs := shop.ScheduledTaskRunArgs(projectRoot, isVerbose)

		cancelCtx, cancel := context.WithCancel(cobraCmd.Context())
		cancelOnTermination(cancelCtx, cancel)

		startRun := func(ctx context.Context) (*exec.Cmd, error) {
			p := cmdExecutor.ConsoleCommand(ctx, runArgs...)
			p.Cmd.Stdout = os.Stdout
			p.Cmd.Stderr = os.Stderr
			p.Cmd.WaitDelay = time.Second
			p.Cmd.Cancel = func() error {
				return gracefulStop(p.Cmd, gracefulStopLimit)
			}

			return p.Cmd, nil
		}

		return shop.RunScheduledTasks(cancelCtx, time.Minute, 30*time.Second, 10*time.Minute, 2, startRun)
	},
}

func init() {
	projectRootCmd.AddCommand(projectScheduledTaskRunnerCmd)
	projectScheduledTaskRunnerCmd.PersistentFlags().Bool("verbose", false, "Enable verbose scheduled task output")
	projectScheduledTaskRunnerCmd.PersistentFlags().Uint("graceful-stop-limit", 0, "Seconds to wait for running tasks to stop gracefully (0 = force-stop immediately)")
}
