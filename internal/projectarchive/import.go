package projectarchive

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"path"

	"github.com/shopware/shopware-cli/internal/sqlshell"
	"github.com/shopware/shopware-cli/logging"
)

// stagingPrefix names the staging directories of an import.
const stagingPrefix = ".shopware-import-"

// ImportOptions configures Import.
type ImportOptions struct {
	// Archive is the path of the archive to import.
	Archive string
	// Contents describes the archive, as returned by Inspect.
	Contents *Contents
	// Environment runs the file transfer commands.
	Environment Environment
	// FileRoots are the roots of the file components the import replaces, as
	// returned by ResolveFileRoots for Contents.FilesReplaced.
	FileRoots FileRoots
	// Database executes the SQL dump, required for the database component.
	Database sqlshell.Execer
}

// Import replaces the project's components with the archive's: the tables
// contained in the dump are recreated, and the media and thumbnail directories
// of the public filesystem and the whole private filesystem are replaced with
// the archived files. File components missing from an archive with a database
// are emptied, see Components.FilesToEmpty.
//
// The files are extracted into staging directories first, so an unwritable
// target or a corrupt archive fails before the database is touched. They
// replace the existing files only after the database import, which keeps the
// window in which database and files disagree short.
func Import(ctx context.Context, opts ImportOptions) (err error) {
	staged, err := stageImportFiles(ctx, opts)
	defer func() {
		// Also clean up after a cancellation, the staging directories hold a
		// copy of the files.
		cleanupCtx := context.WithoutCancel(ctx)
		for _, s := range staged {
			if cleanupErr := s.cleanup(cleanupCtx, opts.Environment); cleanupErr != nil {
				logging.FromContext(ctx).Warnf("Cannot remove the staging directory %s/%s/%s: %v", s.root, s.dirs[0], s.staging, cleanupErr)
			}
		}
	}()
	if err != nil {
		return err
	}

	if opts.Contents.Database {
		if err := importDatabase(ctx, opts); err != nil {
			return err
		}
	}

	for _, s := range staged {
		if s.empty {
			logging.FromContext(ctx).Infof("Removing %s files, as the imported database has no rows for them", s.name)
		} else {
			logging.FromContext(ctx).Infof("Replacing %s files", s.name)
		}
		if err := s.swap(ctx, opts.Environment); err != nil {
			return fmt.Errorf("cannot replace %s files: %w", s.name, err)
		}
		for _, m := range s.ownerMismatches {
			dir := path.Join(s.root, m.dir)
			logging.FromContext(ctx).Warnf("Imported %s files in %s belong to %s instead of %s like the directory, so the web server may not be able to change them. Fix it as root, e.g. with \"chown -R %s %s\"", s.name, dir, m.owner, m.expected, m.expected, dir)
		}
	}

	return nil
}

// stageImportFiles extracts the archived files and stages empty directories
// for the file components to empty. It returns the staged components also on
// error, so their staging directories can be removed.
func stageImportFiles(ctx context.Context, opts ImportOptions) ([]stagedFiles, error) {
	staging := stagingPrefix + rand.Text()
	var staged []stagedFiles

	err := walkArchive(opts.Archive, func(entry string, r io.Reader) error {
		for _, fc := range fileComponents {
			if entry != fc.entry {
				continue
			}
			root, err := opts.FileRoots.root(fc)
			if err != nil {
				return err
			}
			logging.FromContext(ctx).Infof("Extracting %s files", fc.name)
			s, err := stageFiles(ctx, opts.Environment, stagedFiles{fileComponent: fc, root: root, staging: staging}, r)
			staged = append(staged, s)
			if err != nil {
				return fmt.Errorf("cannot extract %s files: %w", fc.name, err)
			}
		}

		return nil
	})
	if err != nil {
		return staged, err
	}

	toEmpty := opts.Contents.FilesToEmpty()
	for _, fc := range fileComponents {
		if !fc.selected(toEmpty) {
			continue
		}
		root, err := opts.FileRoots.root(fc)
		if err != nil {
			return staged, err
		}
		s, err := stageEmptyFiles(ctx, opts.Environment, stagedFiles{fileComponent: fc, root: root, staging: staging})
		staged = append(staged, s)
		if err != nil {
			return staged, fmt.Errorf("cannot prepare removing %s files: %w", fc.name, err)
		}
	}

	return staged, nil
}

// importDatabase executes the archived SQL dump.
func importDatabase(ctx context.Context, opts ImportOptions) error {
	return walkArchive(opts.Archive, func(entry string, r io.Reader) error {
		if entry != databaseEntry {
			return nil
		}
		logging.FromContext(ctx).Infof("Importing database")
		if _, err := sqlshell.ExecuteStream(ctx, opts.Database, r, nil); err != nil {
			return fmt.Errorf("cannot import database: %w", err)
		}

		return nil
	})
}
