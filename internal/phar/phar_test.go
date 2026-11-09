package phar

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testStub = "<?php require 'phar://' . __FILE__ . '/index.php'; __HALT_COMPILER();"

func TestWriteExecutesWithPHP(t *testing.T) {
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP is required for the compatibility test")
	}

	files := map[string][]byte{
		"index.php": []byte(`<?php
require __DIR__ . '/lib/message.php';
echo file_get_contents(__DIR__ . '/data.bin');
echo strlen(file_get_contents(__DIR__ . '/empty.txt'));
$archive = new Phar(Phar::running(false));
echo $archive->getSignature()['hash_type'];
echo $archive['data.bin']->isCompressed() ? 'compressed' : 'plain';
`),
		"lib/message.php": []byte("<?php echo 'hello';"),
		"data.bin":        {0, 255, 1, 128},
		"empty.txt":       {},
	}
	for _, ending := range []string{"", " ?>", " ?>\r\n"} {
		t.Run("ending="+ending, func(t *testing.T) {
			var archive bytes.Buffer
			require.NoError(t, Write(&archive, testStub+ending, files))
			name := filepath.Join(t.TempDir(), "test.phar")
			require.NoError(t, os.WriteFile(name, archive.Bytes(), 0o644))
			output, err := exec.CommandContext(t.Context(), php, name).CombinedOutput()
			require.NoError(t, err, "%s", output)
			assert.Equal(t, "hello\x00\xff\x01\x800SHA-256plain", string(output))

			// Changing only the stub leaves file checksums intact but invalidates
			// the archive signature, so PHP must refuse to load it.
			tampered := bytes.Replace(archive.Bytes(), []byte("<?php "), []byte("<?php  "), 1)
			require.NoError(t, os.WriteFile(name, tampered, 0o644))
			_, err = exec.CommandContext(t.Context(), php, name).CombinedOutput()
			assert.Error(t, err)
		})
	}
}

func TestWriteDeterministic(t *testing.T) {
	var first, second bytes.Buffer
	require.NoError(t, Write(&first, testStub, map[string][]byte{"b": {2}, "a": {1}}))
	require.NoError(t, Write(&second, testStub, map[string][]byte{"a": {1}, "b": {2}}))
	assert.Equal(t, first.Bytes(), second.Bytes())
}

func TestWriteInvalidInput(t *testing.T) {
	for i, name := range []string{"", ".", "../escape", "/absolute", "a/../b", "a//b", "a\\b", "a\x00b", "C:/file", "dir/", strings.Repeat("a", 1024*1024)} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			var out bytes.Buffer
			assert.Error(t, Write(&out, testStub, map[string][]byte{name: {}}))
			assert.Empty(t, out.Bytes())
		})
	}
	assert.Error(t, Write(io.Discard, "<?php", map[string][]byte{"a": {}}))
	assert.Error(t, Write(io.Discard, testStub, nil))
}

type failingWriter struct {
	err error
}

func (w failingWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestWritePropagatesError(t *testing.T) {
	err := errors.New("output unavailable")
	assert.ErrorIs(t, Write(failingWriter{err}, testStub, map[string][]byte{"a": {1}}), err)
}
