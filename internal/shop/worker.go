package shop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/shopware/shopware-cli/logging"
)

// WorkerQueue is a single entry of a worker spec like "async:5".
type WorkerQueue struct {
	Name  string
	Count int
}

// WorkerJob is a single consumer process to be supervised.
type WorkerJob struct {
	Queues       []string
	ConsumerName string
}

// WorkerConfig holds the shared messenger:consume options for all workers.
type WorkerConfig struct {
	MemoryLimit   string
	TimeLimit     string
	FailureLimit  uint
	MessagesLimit uint
	Verbose       bool
}

// StartWorkerFunc creates and fully configures the consumer process for a job
// (stdout, env, graceful stop). It must not start it.
type StartWorkerFunc func(ctx context.Context, job WorkerJob) (*exec.Cmd, error)

// ParseWorkerSpec parses a spec like "async:5,mail:5" into worker queues.
// The count is optional and defaults to 1, so "mail" equals "mail:1".
func ParseWorkerSpec(spec string) ([]WorkerQueue, error) {
	entries := strings.Split(spec, ",")
	queues := make([]WorkerQueue, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("worker spec %q contains an empty entry", spec)
		}

		parts := strings.Split(entry, ":")
		if len(parts) > 2 {
			return nil, fmt.Errorf("worker spec entry %q is invalid, expected format queue:count", entry)
		}

		name := strings.TrimSpace(parts[0])
		if name == "" {
			return nil, fmt.Errorf("worker spec entry %q is missing a queue name", entry)
		}

		count := 1
		if len(parts) == 2 {
			var err error
			count, err = strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("worker spec entry %q has an invalid count: %w", entry, err)
			}
		}

		if count < 1 {
			return nil, fmt.Errorf("worker spec entry %q count must be at least 1", entry)
		}

		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("worker spec contains duplicate queue %q", name)
		}
		seen[name] = struct{}{}

		queues = append(queues, WorkerQueue{Name: name, Count: count})
	}

	return queues, nil
}

// DefaultWorkerQueues returns the default queues to consume based on the
// Shopware version of the project.
func DefaultWorkerQueues(projectRoot string) []string {
	if is, _ := IsShopwareVersion(projectRoot, ">=6.5.7"); is {
		return []string{"async", "failed", "low_priority"}
	} else if is, _ := IsShopwareVersion(projectRoot, ">=6.5"); is {
		return []string{"async", "failed"}
	}

	return nil
}

// ConsumeArgs builds the messenger:consume arguments for the given queues.
func (c WorkerConfig) ConsumeArgs(queues []string) []string {
	args := []string{
		"messenger:consume",
		fmt.Sprintf("--memory-limit=%s", c.MemoryLimit),
		fmt.Sprintf("--time-limit=%s", c.TimeLimit),
		fmt.Sprintf("--failure-limit=%d", c.FailureLimit),
	}

	if c.MessagesLimit > 0 {
		args = append(args, fmt.Sprintf("--limit=%d", c.MessagesLimit))
	}

	args = append(args, queues...)

	if c.Verbose {
		args = append(args, "-vvv")
	}

	return args
}

// PlanWorkers expands either a worker spec ("async:5,mail:5") or the legacy
// worker amount into concrete worker jobs.
func PlanWorkers(spec string, amount int, defaultQueues []string) ([]WorkerJob, error) {
	baseName := fmt.Sprintf("shopware-cli-%d", os.Getpid())

	if spec == "" {
		if amount < 1 {
			return nil, fmt.Errorf("worker amount must be at least 1")
		}

		jobs := make([]WorkerJob, 0, amount)
		for i := 0; i < amount; i++ {
			jobs = append(jobs, WorkerJob{
				Queues:       defaultQueues,
				ConsumerName: fmt.Sprintf("%s-%d", baseName, i),
			})
		}

		return jobs, nil
	}

	workerQueues, err := ParseWorkerSpec(spec)
	if err != nil {
		return nil, err
	}

	total := 0
	for _, q := range workerQueues {
		total += q.Count
	}

	jobs := make([]WorkerJob, 0, total)
	for _, q := range workerQueues {
		for i := 0; i < q.Count; i++ {
			jobs = append(jobs, WorkerJob{
				Queues:       []string{q.Name},
				ConsumerName: fmt.Sprintf("%s-%s-%d", baseName, q.Name, i),
			})
		}
	}

	return jobs, nil
}

// workerRestartInterval throttles consumer restarts per worker slot.
var workerRestartInterval = 10 * time.Second

// RunWorkers supervises one restart-loop goroutine per job. Crashed consumers
// are restarted, throttled by a per-job rate limiter of one start per 10
// seconds. It blocks until the context is cancelled and all consumers have
// stopped.
func RunWorkers(ctx context.Context, jobs []WorkerJob, start StartWorkerFunc) error {
	if len(jobs) == 0 {
		return errors.New("no worker jobs to run")
	}

	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)

		go func(ctx context.Context, job WorkerJob) {
			defer wg.Done()

			workerRatelimit := rate.NewLimiter(rate.Every(workerRestartInterval), 1)

			for ctx.Err() == nil {
				if err := workerRatelimit.Wait(ctx); err != nil {
					if ctx.Err() != nil {
						break
					}
					logging.FromContext(ctx).Error(err)
					continue
				}

				cmd, err := start(ctx, job)
				if err != nil {
					logging.FromContext(ctx).Error(err)
					continue
				}

				if err := cmd.Run(); err != nil {
					if errors.Is(err, context.Canceled) {
						break
					}
					logging.FromContext(ctx).Error(err)
				}
			}
		}(ctx, job)
	}

	wg.Wait()

	return nil
}
