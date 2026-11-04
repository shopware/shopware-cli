package phar

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"io/fs"
	"strings"
)

//go:embed loader.php
var loader string

// Payload contains an archive and its PHP loader, ready for an executor.
// The loader consumes stdin, writes a temporary archive on the target system,
// and removes it on PHP shutdown (including exit and fatal errors). Cleanup
// cannot run if the remote PHP process crashes or is forcibly killed.
type Payload struct {
	archive []byte
}

// Build packages all regular files in source and generates a stub that requires
// entrypoint. Paths are relative to source; use fs.Sub to select a directory in
// an embed.FS. Building requires neither PHP nor access to the target system.
func Build(source fs.FS, entrypoint string) (*Payload, error) {
	files := make(map[string][]byte)
	if err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("cannot package non-regular file %q", name)
		}
		data, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		files[name] = data
		return nil
	}); err != nil {
		return nil, fmt.Errorf("cannot read phar files: %w", err)
	}
	if _, ok := files[entrypoint]; !ok {
		return nil, fmt.Errorf("cannot find phar entrypoint %q", entrypoint)
	}

	quotedEntry := strings.NewReplacer("\\", "\\\\", "'", "\\'").Replace(entrypoint)
	stub := "<?php require 'phar://' . __FILE__ . '/" + quotedEntry + "'; __HALT_COMPILER();"
	var archive bytes.Buffer
	if err := Write(&archive, stub, files); err != nil {
		return nil, err
	}
	return &Payload{archive: archive.Bytes()}, nil
}

// PHPArgs returns arguments for Executor.PHPCommand or SSHExecutor.RemotePHPCommand.
// args are passed unchanged to the entrypoint as $argv[1:] rather than PHP options.
func (p *Payload) PHPArgs(args ...string) []string {
	return append([]string{"-r", strings.TrimPrefix(loader, "<?php\n"), "--"}, args...)
}

// Stdin returns a fresh reader for each invocation. The archive consumes the
// process's stdin; entrypoint input should be passed as arguments instead.
func (p *Payload) Stdin() io.Reader {
	return bytes.NewReader(p.archive)
}
