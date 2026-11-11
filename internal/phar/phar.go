package phar

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"maps"
	"math"
	"slices"
	"strings"
)

// Write writes a PHAR containing stub and files to w. The caller supplies valid
// PHP ending with __HALT_COMPILER(); (optionally followed by ?>). File names must
// be relative, slash-separated paths. Files have mode 0644 and timestamp zero;
// entries are sorted for reproducible output. No alias or metadata is stored.
// A SHA-256 signature covers the stub, manifest, and file contents.
func Write(w io.Writer, stub string, files map[string][]byte) error {
	stub = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(stub), "?>"))
	if !strings.HasSuffix(stub, "__HALT_COMPILER();") {
		return errors.New("cannot write phar: stub must end with __HALT_COMPILER();")
	}
	stub += " ?>\r\n"
	if len(files) == 0 {
		return errors.New("cannot write phar: at least one file is required")
	}

	names := slices.Sorted(maps.Keys(files))
	manifestSize := uint64(18)
	for _, name := range names {
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00:") {
			return fmt.Errorf("cannot write phar: invalid file path %q", name)
		}
		if uint64(len(files[name])) > math.MaxUint32 {
			return fmt.Errorf("cannot write phar: file %q exceeds 4 GiB limit", name)
		}
		manifestSize += 28 + uint64(len(name))
	}
	if manifestSize > 1024*1024 {
		return errors.New("cannot write phar: manifest exceeds 1 MiB limit")
	}

	// All integers are little-endian except the two-byte API version (1.1.0).
	header := binary.LittleEndian.AppendUint32(nil, uint32(manifestSize))
	header = binary.LittleEndian.AppendUint32(header, uint32(len(names)))
	header = append(header, 0x11, 0x00)
	header = binary.LittleEndian.AppendUint32(header, 0x00010000) // Signature present.
	for range 2 {                                                 // Alias length, metadata length.
		header = binary.LittleEndian.AppendUint32(header, 0)
	}
	for _, name := range names {
		data := files[name]
		header = binary.LittleEndian.AppendUint32(header, uint32(len(name)))
		header = append(header, name...)
		for _, value := range []uint32{
			uint32(len(data)), 0, uint32(len(data)), crc32.ChecksumIEEE(data), 0o644, 0,
		} {
			header = binary.LittleEndian.AppendUint32(header, value)
		}
	}

	parts := []io.Reader{strings.NewReader(stub), bytes.NewReader(header)}
	for _, name := range names {
		parts = append(parts, bytes.NewReader(files[name]))
	}
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, hash), io.MultiReader(parts...)); err != nil {
		return fmt.Errorf("cannot write phar: %w", err)
	}
	trailer := binary.LittleEndian.AppendUint32(hash.Sum(nil), 0x0003) // SHA-256.
	trailer = append(trailer, "GBMB"...)
	if _, err := io.Copy(w, bytes.NewReader(trailer)); err != nil {
		return fmt.Errorf("cannot write phar signature: %w", err)
	}
	return nil
}
