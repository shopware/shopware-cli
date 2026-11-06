package phar

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/payload
var payloadFiles embed.FS

func TestPayloadWithPHP(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP is required for the compatibility test")
	}
	source, err := fs.Sub(payloadFiles, "testdata/payload")
	require.NoError(t, err)
	payload, err := Build(source, "main.php")
	require.NoError(t, err)

	for _, mode := range []string{"return", "exit", "fatal"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "result.json")
			args := []string{marker, mode, "--option", "quotes'\" and spaces", "$HOME; echo nope"}
			cmd := exec.CommandContext(t.Context(), php, append([]string{"-n"}, payload.PHPArgs(args...)...)...)
			cmd.Dir = dir
			cmd.Stdin = payload.Stdin()
			output, err := cmd.CombinedOutput()
			if mode == "return" {
				require.NoError(t, err, "%s", output)
				assert.Equal(t, "done\n", string(output))
			} else {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				if mode == "exit" {
					assert.Equal(t, 7, exitErr.ExitCode())
				}
			}
			data, err := os.ReadFile(marker)
			require.NoError(t, err)
			var result struct {
				Archive string
				Args    []string
				Message string
				Cwd     string
			}
			require.NoError(t, json.Unmarshal(data, &result))
			assert.Equal(t, args[1:], result.Args)
			assert.Equal(t, "embedded PHP", result.Message)
			realDir, err := filepath.EvalSymlinks(dir)
			require.NoError(t, err)
			assert.Equal(t, realDir, result.Cwd)
			require.NotEmpty(t, result.Archive)
			assert.NoFileExists(t, result.Archive)
			assert.NoDirExists(t, filepath.Dir(result.Archive))
		})
	}
	t.Run("invalid archive cleanup", func(t *testing.T) {
		dir := t.TempDir()
		invalid := &Payload{archive: append([]byte(nil), payload.archive...)}
		invalid.archive[len(invalid.archive)-9] ^= 1 // Corrupt the signature.
		cmd := exec.CommandContext(t.Context(), php, append([]string{"-n", "-d", "sys_temp_dir=" + dir}, invalid.PHPArgs()...)...)
		cmd.Stdin = invalid.Stdin()
		_, err := cmd.CombinedOutput()
		require.Error(t, err)
		files, err := os.ReadDir(dir)
		require.NoError(t, err)
		assert.Empty(t, files)
	})
}

func TestBuild(t *testing.T) {
	t.Run("missing entrypoint", func(t *testing.T) {
		_, err := Build(fstest.MapFS{}, "missing.php")
		assert.Error(t, err)
	})
	t.Run("filesystem error", func(t *testing.T) {
		_, err := Build(os.DirFS(filepath.Join(t.TempDir(), "missing")), "main.php")
		assert.ErrorIs(t, err, fs.ErrNotExist)
	})
	t.Run("symlink", func(t *testing.T) {
		_, err := Build(fstest.MapFS{"main.php": {Mode: fs.ModeSymlink}}, "main.php")
		assert.Error(t, err)
	})
	t.Run("reusable payload", func(t *testing.T) {
		payload, err := Build(fstest.MapFS{"main.php": {Data: []byte("<?php echo 'hello';")}}, "main.php")
		require.NoError(t, err)
		first, err := io.ReadAll(payload.Stdin())
		require.NoError(t, err)
		second, err := io.ReadAll(payload.Stdin())
		require.NoError(t, err)
		assert.Equal(t, first, second)
	})
}
