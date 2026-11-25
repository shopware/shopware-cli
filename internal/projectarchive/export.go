package projectarchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// exportFilesLog warns about changes while files are exported. Added files are
// harmless, the exported database does not reference them.
const exportFilesLog = "Exporting %s files: do not replace or delete files until the export finishes, otherwise the archive may miss files the exported database references"

// stagedArchive is the name the archive is written to in the staging
// directory before it is renamed to the output. It must differ from the
// payload entries staged next to it.
const stagedArchive = ".archive"

// ExportOptions configures Export.
type ExportOptions struct {
	// Output is the path of the written archive.
	Output string
	// Components selects what the archive contains.
	Components Components
	// Environment runs the file transfer commands.
	Environment Environment
	// Filesystems are the project's filesystems, required for file components.
	Filesystems Filesystems
	// ShopwareVersion is recorded as the source version.
	ShopwareVersion string
	// CreatedAt is recorded as the creation time, defaults to now.
	CreatedAt time.Time
	// Database configures the database component.
	Database DatabaseExport
}

// DatabaseExport configures the database dump of an export.
type DatabaseExport struct {
	Connection *mysql.Config
	Config     *shop.ConfigDump
	// Options shape the dumped data. Output, compression and the exclusion of
	// rows for files missing in the archive are set by Export.
	Options shop.DumpDatabaseOptions
}

// Export writes an archive with the selected components. The payloads are
// staged in a temporary directory next to the output, as tar entries need
// their size upfront. The archive only appears at Output once it is complete.
func Export(ctx context.Context, opts ExportOptions) (err error) {
	if !opts.Components.Any() {
		return errors.New("cannot export: every component is skipped")
	}

	staging, err := os.MkdirTemp(filepath.Dir(opts.Output), ".shopware-export-*")
	if err != nil {
		return fmt.Errorf("cannot create staging directory: %w", err)
	}
	defer func() { err = errors.Join(err, os.RemoveAll(staging)) }()

	createdAt := opts.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	metadata := Metadata{
		FormatVersion: FormatVersion,
		CreatedAt:     createdAt.UTC().Truncate(time.Second),
		Source:        Source{ShopwareVersion: opts.ShopwareVersion},
	}

	var entries []string

	if opts.Components.Database {
		logging.FromContext(ctx).Infof("Exporting database")
		source, err := exportDatabase(ctx, opts, filepath.Join(staging, databaseEntry))
		if err != nil {
			return err
		}
		metadata.Source.Database = source
		entries = append(entries, databaseEntry)
	}

	if opts.Components.PublicFiles {
		logging.FromContext(ctx).Infof(exportFilesLog, "public")
		if err := exportFiles(ctx, opts.Environment, "public", opts.Filesystems.Public, publicDirectories, filepath.Join(staging, publicFilesEntry)); err != nil {
			return err
		}
		entries = append(entries, publicFilesEntry)
	}

	if opts.Components.PrivateFiles {
		logging.FromContext(ctx).Infof(exportFilesLog, "private")
		if err := exportFiles(ctx, opts.Environment, "private", opts.Filesystems.Private, privateDirectories, filepath.Join(staging, privateFilesEntry)); err != nil {
			return err
		}
		entries = append(entries, privateFilesEntry)
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	archivePath := filepath.Join(staging, stagedArchive)
	if err := writeArchive(archivePath, metadataJSON, staging, entries); err != nil {
		return fmt.Errorf("cannot write archive: %w", err)
	}

	if err := os.Rename(archivePath, opts.Output); err != nil {
		return fmt.Errorf("cannot write archive: %w", err)
	}

	return nil
}

func exportDatabase(ctx context.Context, opts ExportOptions, output string) (*DatabaseSource, error) {
	dumpOpts := opts.Database.Options
	dumpOpts.Output = output
	dumpOpts.Compression = ""
	dumpOpts.SkipPublicFileRows = !opts.Components.PublicFiles
	dumpOpts.SkipPrivateFileRows = !opts.Components.PrivateFiles

	if err := shop.DumpDatabase(ctx, opts.Database.Connection, opts.Database.Config, dumpOpts); err != nil {
		return nil, err
	}

	return databaseSource(ctx, opts.Database.Connection)
}

// databaseSource reads vendor and version of the database server.
func databaseSource(ctx context.Context, cfg *mysql.Config) (*DatabaseSource, error) {
	connector, err := mysql.NewConnector(cfg)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	defer func() { _ = db.Close() }()

	var version string
	if err := db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
		return nil, fmt.Errorf("cannot read the database version: %w", err)
	}

	return parseDatabaseVersion(version), nil
}

// parseDatabaseVersion splits a server version like "10.11.6-MariaDB-1:10.11.6"
// or "8.0.36-0ubuntu0.22.04.1" into vendor and version.
func parseDatabaseVersion(version string) *DatabaseSource {
	vendor := "mysql"
	if strings.Contains(strings.ToLower(version), "mariadb") {
		vendor = "mariadb"
	}

	number, _, _ := strings.Cut(version, "-")

	return &DatabaseSource{Vendor: vendor, Version: number}
}

func exportFiles(ctx context.Context, env Environment, name string, fs Filesystem, dirs []string, output string) error {
	root, err := localRoot(name, fs)
	if err != nil {
		return err
	}

	file, err := os.Create(output)
	if err != nil {
		return err
	}

	err = packFiles(ctx, env, root, dirs, file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("cannot export %s files: %w", name, err)
	}

	return nil
}

// writeArchive writes the outer tar.gz with metadata.json first, so readers
// learn about the archive before its payloads. Only the owner may read it, as
// it can contain credentials, customer data, and private documents.
func writeArchive(path string, metadata []byte, dir string, entries []string) (err error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()

	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	defer func() { err = errors.Join(err, tw.Close(), gz.Close()) }()

	modTime := time.Now()
	if err := tw.WriteHeader(&tar.Header{Name: metadataEntry, Mode: 0o644, Size: int64(len(metadata)), ModTime: modTime}); err != nil {
		return err
	}
	if _, err := tw.Write(metadata); err != nil {
		return err
	}

	for _, entry := range entries {
		if err := addFile(tw, filepath.Join(dir, entry), entry, modTime); err != nil {
			return err
		}
	}

	return nil
}

func addFile(tw *tar.Writer, path, name string, modTime time.Time) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: info.Size(), ModTime: modTime}); err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}
