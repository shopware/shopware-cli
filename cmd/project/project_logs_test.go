package project

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

var stdoutCaptureMu sync.Mutex

func newLogsTestCmd(t *testing.T) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	cmd.Flags().Int("lines", 100, "")
	cmd.Flags().BoolP("follow", "f", false, "")
	cmd.Flags().BoolP("list", "l", false, "")

	return cmd
}

func TestProjectLogsRejectsNegativeLines(t *testing.T) {
	t.Parallel()

	cmd := newLogsTestCmd(t)
	require.NoError(t, cmd.Flags().Set("lines", "-5"))

	fakeExec := &logsFakeExecutor{files: []executor.LogFile{{Name: "prod.log"}}}

	err := runProjectLogs(cmd, nil, fakeExec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--lines")
	assert.False(t, fakeExec.getLogCalled, "GetLog must not be invoked with a negative --lines value")
}

func TestProjectLogsPassesValidLinesToGetLog(t *testing.T) {
	t.Parallel()

	cmd := newLogsTestCmd(t)
	require.NoError(t, cmd.Flags().Set("lines", "50"))

	fakeExec := &logsFakeExecutor{files: []executor.LogFile{{Name: "prod.log"}}}

	require.NoError(t, runProjectLogs(cmd, nil, fakeExec))
	require.True(t, fakeExec.getLogCalled)
	assert.Equal(t, 50, fakeExec.gotLines, "valid --lines values are passed through unchanged")
	assert.Equal(t, "prod.log", fakeExec.gotFile)
}

func TestFormatSize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1 << 10, "1.0 KB"},
		{1536, "1.5 KB"},
		{(1 << 20) - 1, "1024.0 KB"},
		{1 << 20, "1.0 MB"},
		{int64(1.5 * float64(1<<20)), "1.5 MB"},
		{10 * (1 << 20), "10.0 MB"},
	}

	for _, c := range cases {
		assert.Equal(t, c.expected, formatSize(c.bytes), "formatSize(%d)", c.bytes)
	}
}

func TestPrintLogFileListEmpty(t *testing.T) {
	stdout, err := captureStdout(func() error {
		return printLogFileList(nil)
	})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "No log files found.")
}

func TestPrintLogFileListShowsAllFiles(t *testing.T) {
	files := []executor.LogFile{
		{Name: "a.log", Size: 2, ModTime: time.Unix(2000, 0)},
		{Name: "b.log", Size: 2048, ModTime: time.Unix(1000, 0)},
	}

	stdout, err := captureStdout(func() error {
		return printLogFileList(files)
	})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "a.log")
	assert.Contains(t, stdout, "b.log")
	assert.Contains(t, stdout, "2.0 KB")
}

// logsFakeExecutor implements executor.Executor, serving canned log files
// and recording GetLog invocations.
type logsFakeExecutor struct {
	files        []executor.LogFile
	getLogCalled bool
	gotFile      string
	gotLines     int
}

func (f *logsFakeExecutor) AvailableLogFiles(context.Context) ([]executor.LogFile, error) {
	return f.files, nil
}

func (f *logsFakeExecutor) GetLog(_ context.Context, file string, lines int, _ bool, _ io.Writer) error {
	f.getLogCalled = true
	f.gotFile = file
	f.gotLines = lines
	return nil
}

func (f *logsFakeExecutor) ConsoleCommand(ctx context.Context, _ ...string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "true")}
}

func (f *logsFakeExecutor) ComposerCommand(ctx context.Context, _ ...string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "true")}
}

func (f *logsFakeExecutor) PHPCommand(ctx context.Context, _ ...string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "true")}
}

func (f *logsFakeExecutor) NPMCommand(ctx context.Context, _ ...string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "true")}
}

func (f *logsFakeExecutor) NormalizePath(hostPath string) string { return hostPath }
func (f *logsFakeExecutor) Type() string                         { return executor.TypeLocal }
func (f *logsFakeExecutor) WithEnv(map[string]string) executor.Executor {
	return f
}
func (f *logsFakeExecutor) WithRelDir(string) executor.Executor    { return f }
func (f *logsFakeExecutor) StartEnvironment(context.Context) error { return nil }
func (f *logsFakeExecutor) StopEnvironment(context.Context, executor.StopOptions) error {
	return nil
}
func (f *logsFakeExecutor) EnvironmentStatus(context.Context) (bool, error) { return true, nil }
func (f *logsFakeExecutor) AdminAPIClient(context.Context) (*adminSdk.Client, error) {
	return nil, executor.ErrNotSupported
}
func (f *logsFakeExecutor) ShopConfig() *shop.Config { return nil }
func (f *logsFakeExecutor) DatabaseConnection(context.Context) (*executor.DatabaseConnection, error) {
	return nil, executor.ErrNotSupported
}

func captureStdout(fn func() error) (string, error) {
	stdoutCaptureMu.Lock()
	defer stdoutCaptureMu.Unlock()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	runErr := fn()
	_ = w.Close()
	<-done
	os.Stdout = orig
	_ = r.Close()
	return buf.String(), runErr
}
