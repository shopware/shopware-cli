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
	// Filesystems are the project's filesystems, required for file components
	// and the file components emptied by the import.
	Filesystems Filesystems
	// Database executes the SQL dump, required for the database component.
	Database sqlshell.Execer
}

// Import replaces the project's components with the archive's: the tables
// contained in the dump are recreated, and the media and thumbnail directories
// of the public filesystem and the whole private filesystem are emptied before
// the archived files are extracted. File components missing from an archive
// with a database are emptied after the import, see Components.FilesToEmpty.
func Import(ctx context.Context, opts ImportOptions) error {
	components := opts.Contents.Components
	toEmpty := components.FilesToEmpty()

	// Validate the filesystems before the first write.
	var publicRoot, privateRoot string
	var err error
	if components.PublicFiles || toEmpty.PublicFiles {
		if publicRoot, err = localRoot("public", opts.Filesystems.Public); err != nil {
			return err
		}
	}
	if components.PrivateFiles || toEmpty.PrivateFiles {
		if privateRoot, err = localRoot("private", opts.Filesystems.Private); err != nil {
			return err
		}
	}

	err = walkArchive(opts.Archive, func(name string, r io.Reader) error {
		switch name {
		case databaseEntry:
			logging.FromContext(ctx).Infof("Importing database")
			if _, err := sqlshell.ExecuteStream(ctx, opts.Database, r, nil); err != nil {
				return fmt.Errorf("cannot import database: %w", err)
			}
		case publicFilesEntry:
			logging.FromContext(ctx).Infof("Importing public files")
			if err := unpackFiles(ctx, opts.Environment, publicRoot, publicDirectories, r); err != nil {
				return fmt.Errorf("cannot import public files: %w", err)
			}
		case privateFilesEntry:
			logging.FromContext(ctx).Infof("Importing private files")
			if err := unpackFiles(ctx, opts.Environment, privateRoot, privateDirectories, r); err != nil {
				return fmt.Errorf("cannot import private files: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	if toEmpty.PublicFiles {
		logging.FromContext(ctx).Infof("Removing public files, as the imported database has no rows for them")
		if err := emptyFiles(ctx, opts.Environment, publicRoot, publicDirectories); err != nil {
			return fmt.Errorf("cannot remove public files: %w", err)
		}
	}
	if toEmpty.PrivateFiles {
		logging.FromContext(ctx).Infof("Removing private files, as the imported database has no rows for them")
		if err := emptyFiles(ctx, opts.Environment, privateRoot, privateDirectories); err != nil {
			return fmt.Errorf("cannot remove private files: %w", err)
		}
	}

	return nil
}
