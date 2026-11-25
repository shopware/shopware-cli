package projectarchive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shopware/shopware-cli/internal/sqlshell"
	"github.com/shopware/shopware-cli/logging"
)

// ImportOptions configures Import.
type ImportOptions struct {
	// Archive is the path of the archive to import.
	Archive string
	// Components selects what is applied. Each must be present in the archive.
	Components Components
	// Environment runs the file transfer commands.
	Environment Environment
	// Filesystems are the project's filesystems, required for file components.
	Filesystems Filesystems
	// Database executes the SQL dump, required for the database component.
	Database sqlshell.Execer
}

// Import replaces the selected components of the project with the archive's:
// the tables contained in the dump are recreated, and the media and thumbnail
// directories of the public filesystem and the whole private filesystem are
// emptied before the archived files are extracted.
func Import(ctx context.Context, opts ImportOptions) error {
	if !opts.Components.Any() {
		return errors.New("cannot import: every component is skipped")
	}

	// Validate the filesystems before the first write.
	var publicRoot, privateRoot string
	var err error
	if opts.Components.PublicFiles {
		if publicRoot, err = localRoot("public", opts.Filesystems.Public); err != nil {
			return err
		}
	}
	if opts.Components.PrivateFiles {
		if privateRoot, err = localRoot("private", opts.Filesystems.Private); err != nil {
			return err
		}
	}

	var applied Components
	err = walkArchive(opts.Archive, func(name string, r io.Reader) error {
		switch {
		case name == databaseEntry && opts.Components.Database:
			logging.FromContext(ctx).Infof("Importing database")
			if _, err := sqlshell.ExecuteStream(ctx, opts.Database, r, nil); err != nil {
				return fmt.Errorf("cannot import database: %w", err)
			}
			applied.Database = true
		case name == publicFilesEntry && opts.Components.PublicFiles:
			logging.FromContext(ctx).Infof("Importing public files")
			if err := unpackFiles(ctx, opts.Environment, publicRoot, publicDirectories, r); err != nil {
				return fmt.Errorf("cannot import public files: %w", err)
			}
			applied.PublicFiles = true
		case name == privateFilesEntry && opts.Components.PrivateFiles:
			logging.FromContext(ctx).Infof("Importing private files")
			if err := unpackFiles(ctx, opts.Environment, privateRoot, privateDirectories, r); err != nil {
				return fmt.Errorf("cannot import private files: %w", err)
			}
			applied.PrivateFiles = true
		}

		return nil
	})
	if err != nil {
		return err
	}

	if missing := missingComponents(opts.Components, applied); len(missing) > 0 {
		return fmt.Errorf("cannot import archive %s: it does not contain %s", opts.Archive, strings.Join(missing, ", "))
	}

	return nil
}

func missingComponents(wanted, applied Components) []string {
	var missing []string
	if wanted.Database && !applied.Database {
		missing = append(missing, databaseEntry)
	}
	if wanted.PublicFiles && !applied.PublicFiles {
		missing = append(missing, publicFilesEntry)
	}
	if wanted.PrivateFiles && !applied.PrivateFiles {
		missing = append(missing, privateFilesEntry)
	}

	return missing
}
