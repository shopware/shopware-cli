package phar

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

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
			cmd := exec.CommandContext(t.Context(), php, payload.PHPArgs(args...)...)
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
		cmd := exec.CommandContext(t.Context(), php, append([]string{"-d", "sys_temp_dir=" + dir}, invalid.PHPArgs()...)...)
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

func TestPayloadPreservesInteractiveStdin(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP required")
	}
	payload, err := Build(fstest.MapFS{"main.php": {Data: []byte("<?php echo \"READY\\n\"; fflush(STDOUT); echo fgets(STDIN);")}}, "main.php")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, php, payload.PHPArgs()...)
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	defer func() { _ = stdin.Close() }()
	_, err = io.Copy(stdin, payload.Stdin())
	require.NoError(t, err)
	reader := bufio.NewReader(stdout)
	ready, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "READY\n", ready)
	_, err = io.WriteString(stdin, "ACTIVATE\n")
	require.NoError(t, err)
	reply, err := reader.ReadString('\n')
	require.NoError(t, err)
	assert.Equal(t, "ACTIVATE\n", reply)
	require.NoError(t, stdin.Close())
	require.NoError(t, cmd.Wait())
}

func TestPayloadRejectsTruncatedArchive(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP required")
	}
	payload, err := Build(fstest.MapFS{"main.php": {Data: []byte("<?php echo 'executed';")}}, "main.php")
	require.NoError(t, err)
	dir := t.TempDir()
	cmd := exec.CommandContext(t.Context(), php, append([]string{"-d", "sys_temp_dir=" + dir}, payload.PHPArgs()...)...)
	cmd.Stdin = io.LimitReader(payload.Stdin(), int64(len(payload.archive)-1))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	require.Error(t, err)
	assert.Empty(t, string(output), "loader errors must not reach stdout")
	assert.Contains(t, stderr.String(), "Cannot write temporary phar")
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, files)
}
