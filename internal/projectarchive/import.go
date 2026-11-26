package projectarchive

import (
	"context"
	"fmt"
	"io"

	"github.com/shopware/shopware-cli/internal/sqlshell"
	"github.com/shopware/shopware-cli/logging"
)

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
// of the public filesystem and the whole private filesystem are emptied before
// the archived files are extracted. File components missing from an archive
// with a database are emptied after the import, see Components.FilesToEmpty.
func Import(ctx context.Context, opts ImportOptions) error {
	err := walkArchive(opts.Archive, func(name string, r io.Reader) error {
		if name == databaseEntry {
			logging.FromContext(ctx).Infof("Importing database")
			if _, err := sqlshell.ExecuteStream(ctx, opts.Database, r, nil); err != nil {
				return fmt.Errorf("cannot import database: %w", err)
			}
			return nil
		}

		for _, fc := range fileComponents {
			if name != fc.entry {
				continue
			}
			root, err := opts.FileRoots.root(fc)
			if err != nil {
				return err
			}
			logging.FromContext(ctx).Infof("Importing %s files", fc.name)
			if err := unpackFiles(ctx, opts.Environment, root, fc.dirs, r); err != nil {
				return fmt.Errorf("cannot import %s files: %w", fc.name, err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	toEmpty := opts.Contents.FilesToEmpty()
	for _, fc := range fileComponents {
		if !fc.selected(toEmpty) {
			continue
		}
		root, err := opts.FileRoots.root(fc)
		if err != nil {
			return err
		}
		logging.FromContext(ctx).Infof("Removing %s files, as the imported database has no rows for them", fc.name)
		if err := emptyFiles(ctx, opts.Environment, root, fc.dirs); err != nil {
			return fmt.Errorf("cannot remove %s files: %w", fc.name, err)
		}
	}

	return nil
}
