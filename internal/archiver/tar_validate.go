package archiver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// ValidateTarGz checks an archive before extracting it on a deployment host.
// Only regular files, directories, and contained relative symlinks are allowed.
// It consumes the gzip stream, including its checksum, without extracting files.
func ValidateTarGz(ctx context.Context, r io.Reader) (err error) {
	gz, err := gzip.NewReader(ContextReader(ctx, r))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, gz.Close()) }()
	tr := tar.NewReader(gz)
	entries := make(map[string]*tar.Header)
	directories := map[string]bool{".": true}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		name := path.Clean(header.Name)
		if !containedArchivePath(header.Name) || (name == "." && header.Typeflag != tar.TypeDir) {
			return fmt.Errorf("unsafe archive path %q", header.Name)
		}
		if _, exists := entries[name]; exists {
			return fmt.Errorf("duplicate archive path %q", name)
		}
		if header.Mode&0o7000 != 0 {
			return fmt.Errorf("special permission bits on archive entry %q", name)
		}
		switch header.Typeflag {
		case tar.TypeReg, tar.TypeDir:
		case tar.TypeSymlink:
			if header.Linkname == "" || path.IsAbs(header.Linkname) {
				return fmt.Errorf("archive symlink %q must have a relative target", name)
			}
		default:
			return fmt.Errorf("unsupported archive entry %q (type %d)", name, header.Typeflag)
		}
		entries[name] = header
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			directories[dir] = true
		}
		if header.Typeflag == tar.TypeDir {
			directories[name] = true
		}
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return err
	}
	for name, header := range entries {
		if directories[name] && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("archive entry %q is used as a directory", name)
		}
		if header.Typeflag == tar.TypeSymlink {
			if err := validateArchiveLink(name, entries, directories); err != nil {
				return err
			}
		}
	}
	return nil
}

func containedArchivePath(name string) bool {
	clean := path.Clean(name)
	return name != "" && !path.IsAbs(name) && clean != ".." && !strings.HasPrefix(clean, "../") &&
		!strings.Contains("/"+name+"/", "/../") && !strings.ContainsRune(name, '\\')
}

func validateArchiveLink(name string, entries map[string]*tar.Header, directories map[string]bool) error {
	pending := strings.Split(name, "/")
	var resolved []string
	hops := 0
	for len(pending) > 0 {
		part := pending[0]
		pending = pending[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			if len(resolved) == 0 {
				return fmt.Errorf("archive symlink %q points outside the project", name)
			}
			resolved = resolved[:len(resolved)-1]
			continue
		}
		prefix := path.Join(path.Join(resolved...), part)
		entry := entries[prefix]
		if entry != nil && entry.Typeflag == tar.TypeSymlink {
			hops++
			if hops > 40 {
				return fmt.Errorf("archive symlink %q contains a cycle or too many links", name)
			}
			// Resolve symlinks BEFORE processing subsequent ".." components,
			// matching filesystem semantics rather than lexical path cleaning.
			pending = append(strings.Split(entry.Linkname, "/"), pending...)
			continue
		}
		if len(pending) > 0 && !directories[prefix] {
			return fmt.Errorf("archive symlink %q traverses a non-directory %q", name, prefix)
		}
		resolved = append(resolved, part)
	}
	target := path.Join(resolved...)
	if target == "" {
		target = "."
	}
	if entries[target] == nil && !directories[target] {
		return fmt.Errorf("archive symlink %q has missing target %q", name, target)
	}
	return nil
}
