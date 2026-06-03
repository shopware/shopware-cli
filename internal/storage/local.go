package storage

import (
	"io/fs"
	"os"
	"path/filepath"
)

// LocalFile is a single file discovered on disk for a datastore.
type LocalFile struct {
	// AbsPath is the absolute path of the file on disk.
	AbsPath string
	// Key is the path relative to the datastore's LocalBase. It doubles as the
	// object key (before an optional bucket root prefix is prepended).
	Key string
	// Size is the file size in bytes.
	Size int64
}

// ScanResult is the outcome of scanning the local disk for one datastore.
type ScanResult struct {
	Datastore Datastore
	Files     []LocalFile
	TotalSize int64
}

// ScanLocal walks the local directories of the given datastore and returns the
// files that would be migrated. Missing directories are treated as empty, so a
// datastore without any files yields an empty result rather than an error.
func ScanLocal(projectRoot string, ds Datastore) (ScanResult, error) {
	result := ScanResult{Datastore: ds}

	base := filepath.Join(projectRoot, filepath.FromSlash(ds.LocalBase))

	// Directories to walk, relative to base. An empty prefix means the whole base.
	roots := ds.Prefixes
	if len(roots) == 0 {
		roots = []string{""}
	}

	for _, prefix := range roots {
		dir := base
		if prefix != "" {
			dir = filepath.Join(base, filepath.FromSlash(prefix))
		}

		if _, err := os.Stat(dir); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return result, err
		}

		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			// Skip symlinks and other non-regular files.
			if !d.Type().IsRegular() {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return err
			}

			rel, err := filepath.Rel(base, path)
			if err != nil {
				return err
			}

			result.Files = append(result.Files, LocalFile{
				AbsPath: path,
				Key:     filepath.ToSlash(rel),
				Size:    info.Size(),
			})
			result.TotalSize += info.Size()

			return nil
		})
		if err != nil {
			return result, err
		}
	}

	return result, nil
}
