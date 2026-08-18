package project

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectWorkerCmd = &cobra.Command{
	Use:   "worker [amount | queue-spec]",
	Short: "Run multiple Symfony worker in background.",
	Long: `Run multiple Symfony messenger consumers in background.

The first argument is either a worker amount (e.g. "5") or a queue spec
(e.g. "async:5,mail:5") which starts the given amount of consumers per
queue. The count per queue is optional and defaults to 1.`,
	RunE: func(cobraCmd *cobra.Command, args []string) error {
		var projectRoot string
		var err error
		workerAmount := 1
		workerSpec := ""

		isVerbose, _ := cobraCmd.Flags().GetBool("verbose")
		queuesToConsume, _ := cobraCmd.Flags().GetString("queue")
		memoryLimit, _ := cobraCmd.Flags().GetString("memory-limit")
		timeLimit, _ := cobraCmd.Flags().GetString("time-limit")
		gracefulStopLimit, _ := cobraCmd.Flags().GetUint("graceful-stop-limit")
		messagesLimit, _ := cobraCmd.Flags().GetUint("limit")

		if projectRoot, err = findClosestShopwareProject(false); err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cobraCmd, projectRoot)
		if err != nil {
			return err
		}

		if len(args) > 0 {
			if amount, convErr := strconv.Atoi(args[0]); convErr == nil {
				workerAmount = amount
			} else {
				workerSpec = args[0]
				workerAmount = 0
			}
		}

		if workerSpec != "" && queuesToConsume != "" {
			return errors.New("--queue cannot be combined with a queue spec argument")
		}

		if memoryLimit == "" {
			memoryLimit = "512M"
		}

		if timeLimit == "" {
			timeLimit = "120"
		}

		var defaultQueues []string
		if workerSpec == "" {
			if queuesToConsume != "" {
				defaultQueues = strings.Split(queuesToConsume, ",")
			} else {
				defaultQueues = shop.DefaultWorkerQueues(projectRoot)
			}
		}

		jobs, err := shop.PlanWorkers(workerSpec, workerAmount, defaultQueues)
		if err != nil {
			return err
		}

		consumeConfig := shop.WorkerConfig{
			MemoryLimit:   memoryLimit,
			TimeLimit:     timeLimit,
			FailureLimit:  5,
			MessagesLimit: messagesLimit,
			Verbose:       isVerbose,
		}

		cancelCtx, cancel := context.WithCancel(cobraCmd.Context())
		cancelOnTermination(cancelCtx, cancel)

		startWorker := func(ctx context.Context, job shop.WorkerJob) (*exec.Cmd, error) {
			p := cmdExecutor.ConsoleCommand(ctx, consumeConfig.ConsumeArgs(job.Queues)...)
			p.Cmd.Stdout = os.Stdout
			p.Cmd.Stderr = os.Stderr
			p.Cmd.Env = append(os.Environ(), "MESSENGER_CONSUMER_NAME="+job.ConsumerName)
			p.Cmd.WaitDelay = time.Second
			p.Cmd.Cancel = func() error {
				return gracefulStop(p.Cmd, gracefulStopLimit)
			}

			return p.Cmd, nil
		}

		return shop.RunWorkers(cancelCtx, jobs, startWorker)
	},
}

func init() {
	projectRootCmd.AddCommand(projectWorkerCmd)
	projectWorkerCmd.PersistentFlags().Bool("verbose", false, "Enable verbose output")
	projectWorkerCmd.PersistentFlags().String("queue", "", "Queues to consume")
	projectWorkerCmd.PersistentFlags().String("memory-limit", "", "Memory Limit")
	projectWorkerCmd.PersistentFlags().String("time-limit", "", "Time Limit")
	projectWorkerCmd.PersistentFlags().Uint("graceful-stop-limit", 0, "Graceful Stop Limit")
	projectWorkerCmd.PersistentFlags().Uint("limit", 0, "Messages Limit")
}

func cancelOnTermination(ctx context.Context, cancel context.CancelFunc) {
	logging.FromContext(ctx).Infof("setting up a signal handler")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-s
		logging.FromContext(ctx).Infof("received signal %v\n", sig.String())
		cancel()
	}()
}
