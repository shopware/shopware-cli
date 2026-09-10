package archiver

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// WriteTarGz writes the contents of dir without an enclosing directory. It
// preserves permissions and relative symlinks, but never follows symlinks.
// exclude receives slash-separated paths relative to dir.
func WriteTarGz(ctx context.Context, w io.Writer, dir string, exclude func(string) bool) (err error) {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	defer func() {
		err = errors.Join(err, tw.Close(), gz.Close())
	}()

	return filepath.WalkDir(dir, func(name string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, name)
		if err != nil || rel == "." {
			return err
		}
		if exclude != nil && exclude(filepath.ToSlash(rel)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = ContainedSymlink(dir, name)
			if err != nil {
				return err
			}
		} else if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("cannot archive special file %q", rel)
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		// Ownership on the build machine has no meaning on the deployment host.
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(name)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		_, err = io.Copy(tw, ContextReader(ctx, file))
		return err
	})
}

// ContainedSymlink rejects absolute, broken, and escaping links. Checking the
// resolved target also catches escapes through intermediate directory symlinks.
func ContainedSymlink(root, name string) (string, error) {
	link, err := os.Readlink(name)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(link) {
		return "", fmt.Errorf("symlink %q must use a relative target", name)
	}
	target, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", fmt.Errorf("resolve symlink %q: %w", name, err)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, target)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("symlink %q points outside the project", name)
	}
	return filepath.ToSlash(link), nil
}

// ContextReader makes copying large files interruptible between reads.
func ContextReader(ctx context.Context, r io.Reader) io.Reader {
	return readerFunc(func(p []byte) (int, error) {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		return r.Read(p)
	})
}

type readerFunc func([]byte) (int, error)

func (r readerFunc) Read(p []byte) (int, error) {
	return r(p)
}
