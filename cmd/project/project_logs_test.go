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

func TestParseLogFilesSortsByModTime(t *testing.T) {
	t.Parallel()

	out := []byte(`[{"name":"older.log","size":13,"mtime":1000},{"name":"newer.log","size":5,"mtime":2000}]`)

	files, err := parseLogFiles(out)
	require.NoError(t, err)
	require.Len(t, files, 2)

	assert.Equal(t, "newer.log", files[0].name)
	assert.Equal(t, int64(5), files[0].size)
	assert.Equal(t, int64(2000), files[0].modTime.Unix())
	assert.Equal(t, "older.log", files[1].name)
}

func TestParseLogFilesEmpty(t *testing.T) {
	t.Parallel()

	files, err := parseLogFiles([]byte(`[]`))
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestParseLogFilesInvalid(t *testing.T) {
	t.Parallel()

	_, err := parseLogFiles([]byte(`not json`))
	assert.Error(t, err)
}

func TestPrintLogFileListEmpty(t *testing.T) {
	stdout, err := captureStdout(func() error {
		return printLogFileList(nil)
	})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "No log files found.")
}

func TestPrintLogFileListShowsAllFiles(t *testing.T) {
	files := []logFileInfo{
		{name: "a.log", size: 2, modTime: time.Unix(2000, 0)},
		{name: "b.log", size: 2048, modTime: time.Unix(1000, 0)},
	}

	stdout, err := captureStdout(func() error {
		return printLogFileList(files)
	})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "a.log")
	assert.Contains(t, stdout, "b.log")
	assert.Contains(t, stdout, "2.0 KB")
}

func TestFindLogFilesUsesExecutor(t *testing.T) {
	t.Parallel()

	fakeExec := &logsFakeExecutor{
		php: func(ctx context.Context, args ...string) *executor.Process {
			assert.Equal(t, "-r", args[0])
			assert.Contains(t, args[1], "var/log/*.log")
			return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", `printf '%s' '[{"name":"prod.log","size":10,"mtime":2000}]'`)}
		},
	}

	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())

	files, err := findLogFiles(cmd, fakeExec)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "prod.log", files[0].name)
}

// logsFakeExecutor implements executor.Executor; every command but the
// scripted PHPCommand exits successfully without output.
type logsFakeExecutor struct {
	php func(ctx context.Context, args ...string) *executor.Process
}

func trueProcess(ctx context.Context) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "true")}
}

func (f *logsFakeExecutor) PHPCommand(ctx context.Context, args ...string) *executor.Process {
	return f.php(ctx, args...)
}

func (f *logsFakeExecutor) ConsoleCommand(ctx context.Context, _ ...string) *executor.Process {
	return trueProcess(ctx)
}

func (f *logsFakeExecutor) ComposerCommand(ctx context.Context, _ ...string) *executor.Process {
	return trueProcess(ctx)
}

func (f *logsFakeExecutor) NPMCommand(ctx context.Context, _ ...string) *executor.Process {
	return trueProcess(ctx)
}

func (f *logsFakeExecutor) Command(ctx context.Context, _ string, _ ...string) *executor.Process {
	return trueProcess(ctx)
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
