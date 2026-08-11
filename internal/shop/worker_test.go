package shop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseWorkerSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    []WorkerQueue
		wantErr bool
	}{
		{
			name: "single queue with count",
			spec: "async:5",
			want: []WorkerQueue{{Name: "async", Count: 5}},
		},
		{
			name: "multiple queues with counts",
			spec: "async:5,mail:5",
			want: []WorkerQueue{{Name: "async", Count: 5}, {Name: "mail", Count: 5}},
		},
		{
			name: "count defaults to one",
			spec: "mail",
			want: []WorkerQueue{{Name: "mail", Count: 1}},
		},
		{
			name: "mixed with and without count",
			spec: "async:2,mail,scheduled_task:3",
			want: []WorkerQueue{
				{Name: "async", Count: 2},
				{Name: "mail", Count: 1},
				{Name: "scheduled_task", Count: 3},
			},
		},
		{
			name: "whitespace is trimmed",
			spec: " async : 2 , mail : 1 ",
			want: []WorkerQueue{{Name: "async", Count: 2}, {Name: "mail", Count: 1}},
		},
		{name: "empty spec", spec: "", wantErr: true},
		{name: "empty entry", spec: "async,,mail", wantErr: true},
		{name: "missing name", spec: ":5", wantErr: true},
		{name: "missing count", spec: "async:", wantErr: true},
		{name: "non numeric count", spec: "async:x", wantErr: true},
		{name: "zero count", spec: "async:0", wantErr: true},
		{name: "negative count", spec: "async:-1", wantErr: true},
		{name: "too many separators", spec: "async:5:2", wantErr: true},
		{name: "duplicate queue", spec: "async:1,async:2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseWorkerSpec(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDefaultWorkerQueues(t *testing.T) {
	t.Run("no composer.json", func(t *testing.T) {
		assert.Nil(t, DefaultWorkerQueues(t.TempDir()))
	})
}

func TestWorkerConfigConsumeArgs(t *testing.T) {
	t.Run("minimal", func(t *testing.T) {
		cfg := WorkerConfig{MemoryLimit: "512M", TimeLimit: "120", FailureLimit: 5}

		assert.Equal(t, []string{
			"messenger:consume",
			"--memory-limit=512M",
			"--time-limit=120",
			"--failure-limit=5",
			"async", "failed",
		}, cfg.ConsumeArgs([]string{"async", "failed"}))
	})

	t.Run("with message limit and verbose", func(t *testing.T) {
		cfg := WorkerConfig{MemoryLimit: "256M", TimeLimit: "60", FailureLimit: 3, MessagesLimit: 10, Verbose: true}

		assert.Equal(t, []string{
			"messenger:consume",
			"--memory-limit=256M",
			"--time-limit=60",
			"--failure-limit=3",
			"--limit=10",
			"mail",
			"-vvv",
		}, cfg.ConsumeArgs([]string{"mail"}))
	})
}

func TestPlanWorkers(t *testing.T) {
	baseName := fmt.Sprintf("shopware-cli-%d", os.Getpid())

	t.Run("legacy amount mode", func(t *testing.T) {
		jobs, err := PlanWorkers("", 3, []string{"async", "failed"})
		require.NoError(t, err)
		require.Len(t, jobs, 3)

		for i, job := range jobs {
			assert.ElementsMatch(t, []string{"async", "failed"}, job.Queues)
			assert.Equal(t, fmt.Sprintf("%s-%d", baseName, i), job.ConsumerName)
		}
	})

	t.Run("legacy amount must be positive", func(t *testing.T) {
		_, err := PlanWorkers("", 0, []string{"async"})
		require.Error(t, err)
	})

	t.Run("spec mode", func(t *testing.T) {
		jobs, err := PlanWorkers("async:2,mail:1", 1, nil)
		require.NoError(t, err)
		require.Len(t, jobs, 3)

		assert.Equal(t, []string{"async"}, jobs[0].Queues)
		assert.Equal(t, baseName+"-async-0", jobs[0].ConsumerName)
		assert.Equal(t, []string{"async"}, jobs[1].Queues)
		assert.Equal(t, baseName+"-async-1", jobs[1].ConsumerName)
		assert.Equal(t, []string{"mail"}, jobs[2].Queues)
		assert.Equal(t, baseName+"-mail-0", jobs[2].ConsumerName)
	})

	t.Run("invalid spec", func(t *testing.T) {
		_, err := PlanWorkers("async:0", 1, nil)
		require.Error(t, err)
	})
}

func TestRunWorkers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires the true binary")
	}

	t.Run("no jobs", func(t *testing.T) {
		require.Error(t, RunWorkers(t.Context(), nil, nil))
	})

	t.Run("jobs restart independently", func(t *testing.T) {
		oldInterval := workerRestartInterval
		workerRestartInterval = 10 * time.Millisecond
		t.Cleanup(func() { workerRestartInterval = oldInterval })

		var mu sync.Mutex
		counts := map[string]int{}

		start := func(ctx context.Context, job WorkerJob) (*exec.Cmd, error) {
			mu.Lock()
			counts[job.ConsumerName]++
			mu.Unlock()

			return exec.CommandContext(ctx, "true"), nil
		}

		ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
		defer cancel()

		jobs := []WorkerJob{
			{Queues: []string{"async"}, ConsumerName: "async"},
			{Queues: []string{"mail"}, ConsumerName: "mail"},
		}

		require.NoError(t, RunWorkers(ctx, jobs, start))

		mu.Lock()
		defer mu.Unlock()

		// With a per-job limiter each slot restarts once per interval (~50
		// times in the window). A shared limiter would only manage ~25 per
		// job, so this also guards against regression to a shared bucket.
		assert.GreaterOrEqual(t, counts["async"], 35)
		assert.GreaterOrEqual(t, counts["mail"], 35)
	})

	t.Run("start errors are retried", func(t *testing.T) {
		oldInterval := workerRestartInterval
		workerRestartInterval = 10 * time.Millisecond
		t.Cleanup(func() { workerRestartInterval = oldInterval })

		var mu sync.Mutex
		attempts := 0

		start := func(_ context.Context, _ WorkerJob) (*exec.Cmd, error) {
			mu.Lock()
			attempts++
			mu.Unlock()

			return nil, fmt.Errorf("boom")
		}

		ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
		defer cancel()

		require.NoError(t, RunWorkers(ctx, []WorkerJob{{Queues: []string{"async"}, ConsumerName: "async"}}, start))

		mu.Lock()
		defer mu.Unlock()
		assert.GreaterOrEqual(t, attempts, 2)
	})
}
