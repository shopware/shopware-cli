package project

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func TestFindLogFilesMissingDirectory(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	_, err := findLogFiles(filepath.Join(tmp, "does-not-exist"))
	assert.Error(t, err)
}

func TestFindLogFilesEmptyDirectory(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	files, err := findLogFiles(tmp)
	assert.NoError(t, err)
	assert.Empty(t, files)
}

func TestFindLogFilesFiltersAndSortsByModTime(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	writeFile := func(name, content string) string {
		p := filepath.Join(tmp, name)
		assert.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		return p
	}

	older := writeFile("older.log", "older content")
	newer := writeFile("newer.log", "newer")
	writeFile("notes.txt", "ignore me")
	assert.NoError(t, os.Mkdir(filepath.Join(tmp, "subdir.log"), 0o755))

	now := time.Now()
	assert.NoError(t, os.Chtimes(older, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
	assert.NoError(t, os.Chtimes(newer, now, now))

	files, err := findLogFiles(tmp)
	assert.NoError(t, err)
	assert.Len(t, files, 2)

	assert.Equal(t, "newer.log", files[0].name)
	assert.Equal(t, "older.log", files[1].name)
	assert.Equal(t, int64(len("newer")), files[0].size)
	assert.Equal(t, int64(len("older content")), files[1].size)
	assert.Equal(t, filepath.Join(tmp, "newer.log"), files[0].path)
}

func TestListLogFilesEmpty(t *testing.T) {
	tmp := t.TempDir()
	stdout, err := captureStdout(func() error {
		return listLogFiles(tmp)
	})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "No log files found.")
}

func TestListLogFilesMissingDir(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	err := listLogFiles(filepath.Join(tmp, "missing"))
	assert.Error(t, err)
}

func TestListLogFilesShowsAllFiles(t *testing.T) {
	tmp := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(tmp, "a.log"), []byte("hi"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(tmp, "b.log"), make([]byte, 2048), 0o644))

	stdout, err := captureStdout(func() error {
		return listLogFiles(tmp)
	})
	assert.NoError(t, err)
	assert.Contains(t, stdout, "a.log")
	assert.Contains(t, stdout, "b.log")
	assert.Contains(t, stdout, "2.0 KB")
}

func TestPrintLastLinesShortFile(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "short.log")
	assert.NoError(t, os.WriteFile(p, []byte("line1\nline2\nline3\n"), 0o644))

	stdout, err := captureStdout(func() error {
		return printLastLines(p, 100)
	})
	assert.NoError(t, err)
	assert.Equal(t, "line1\nline2\nline3\n", stdout)
}

func TestPrintLastLinesTruncatesToLastN(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "long.log")

	var sb strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&sb, "line%02d\n", i)
	}
	assert.NoError(t, os.WriteFile(p, []byte(sb.String()), 0o644))

	stdout, err := captureStdout(func() error {
		return printLastLines(p, 5)
	})
	assert.NoError(t, err)
	assert.Equal(t, "line45\nline46\nline47\nline48\nline49\n", stdout)
}

func TestPrintLastLinesExactNLines(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "exact.log")
	assert.NoError(t, os.WriteFile(p, []byte("a\nb\nc\n"), 0o644))

	stdout, err := captureStdout(func() error {
		return printLastLines(p, 3)
	})
	assert.NoError(t, err)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestPrintLastLinesMissingFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	err := printLastLines(filepath.Join(tmp, "missing.log"), 10)
	assert.Error(t, err)
}

func TestPrintLastLinesRingBufferOverflow(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "ring.log")

	var sb strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&sb, "L%04d\n", i)
	}
	assert.NoError(t, os.WriteFile(p, []byte(sb.String()), 0o644))

	stdout, err := captureStdout(func() error {
		return printLastLines(p, 3)
	})
	assert.NoError(t, err)
	assert.Equal(t, "L1997\nL1998\nL1999\n", stdout)
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
