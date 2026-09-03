package executor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogFilesSortsByModTime(t *testing.T) {
	t.Parallel()

	out := []byte(`[{"name":"older.log","size":13,"mtime":1000},{"name":"newer.log","size":5,"mtime":2000}]`)

	files, err := parseLogFiles(out)
	require.NoError(t, err)
	require.Len(t, files, 2)

	assert.Equal(t, "newer.log", files[0].Name)
	assert.Equal(t, int64(5), files[0].Size)
	assert.Equal(t, int64(2000), files[0].ModTime.Unix())
	assert.Equal(t, "older.log", files[1].Name)
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

func TestLocalLogFilesMissingDirectory(t *testing.T) {
	t.Parallel()

	_, err := localLogFiles(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

func TestLocalLogFilesEmptyDirectory(t *testing.T) {
	t.Parallel()

	files, err := localLogFiles(t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestLocalLogFilesFiltersAndSortsByModTime(t *testing.T) {
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

	files, err := localLogFiles(tmp)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	assert.Equal(t, "newer.log", files[0].Name)
	assert.Equal(t, "older.log", files[1].Name)
	assert.Equal(t, int64(len("newer")), files[0].Size)
	assert.Equal(t, int64(len("older content")), files[1].Size)
}

func TestTailArgs(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"-n", "100", "var/log/prod.log"}, tailArgs("var/log/prod.log", 100, false))
	assert.Equal(t, []string{"-n", "5", "-f", "var/log/prod.log"}, tailArgs("var/log/prod.log", 5, true))
}

func TestLocalExecutorAvailableLogFiles(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "var", "log"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "var", "log", "prod.log"), []byte("line\n"), 0o644))

	files, err := NewLocal(tmp).AvailableLogFiles(t.Context())
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "prod.log", files[0].Name)
}

func TestLocalExecutorGetLog(t *testing.T) {
	t.Parallel()

	p := NewLocal("/project").GetLog(t.Context(), "prod.log", 50, true)

	assert.Equal(t, "tail", filepath.Base(p.Cmd.Path))
	assert.Equal(t, []string{"tail", "-n", "50", "-f", filepath.Join("/project", "var", "log", "prod.log")}, p.Cmd.Args)
}
