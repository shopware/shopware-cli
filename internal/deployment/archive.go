package deployment

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// defaultExcludes are never uploaded to the deployment target.
var defaultExcludes = []string{
	".git",
	".github",
	".idea",
	".vscode",
	"node_modules",
	"var/cache",
	"var/log",
	".shopware-project.local.yml",
	".shopware-project.local.yaml",
}

// writeProjectArchive writes the project directory as a gzipped tarball to w.
// Paths in exclude (and sharedPaths, which live in the shared directory on the
// target) are skipped. All paths are relative to root and use forward slashes.
func writeProjectArchive(w io.Writer, root string, exclude []string) error {
	gzWriter := gzip.NewWriter(w)
	tarWriter := tar.NewWriter(gzWriter)

	excluded := make([]string, 0, len(defaultExcludes)+len(exclude))
	for _, e := range slices.Concat(defaultExcludes, exclude) {
		excluded = append(excluded, filepath.ToSlash(strings.Trim(e, "/")))
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		relPath = filepath.ToSlash(relPath)

		if slices.Contains(excluded, relPath) {
			if entry.IsDir() {
				return filepath.SkipDir
			}

			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		var linkTarget string
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return err
			}
		} else if !info.Mode().IsRegular() && !info.IsDir() {
			// skip sockets, devices, pipes
			return nil
		}

		header, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return err
		}

		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.Mode().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}

			if _, err := io.Copy(tarWriter, file); err != nil {
				_ = file.Close()
				return fmt.Errorf("cannot archive %s: %w", relPath, err)
			}

			if err := file.Close(); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if err := tarWriter.Close(); err != nil {
		return err
	}

	return gzWriter.Close()
}
