package project

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/executor"
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
