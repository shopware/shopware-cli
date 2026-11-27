package projectarchive

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/executor"
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
	// FileRoots are the roots of the selected file components, as returned by
	// ResolveFileRoots.
	FileRoots FileRoots
	// ShopwareVersion is recorded as the source version.
	ShopwareVersion string
	// CreatedAt is recorded as the creation time, defaults to now.
	CreatedAt time.Time
	// Database configures the database component.
	Database DatabaseExport
}

// DatabaseExport configures the database dump of an export.
type DatabaseExport struct {
	Connection *executor.DatabaseConnection
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

	for _, fc := range fileComponents {
		if !fc.selected(opts.Components) {
			continue
		}
		logging.FromContext(ctx).Infof(exportFilesLog, fc.name)
		if err := exportFiles(ctx, opts.Environment, fc, opts.FileRoots, filepath.Join(staging, fc.entry)); err != nil {
			return err
		}
		entries = append(entries, fc.entry)
	}

	metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}

	archivePath := filepath.Join(staging, stagedArchive)
	if err := writeArchive(ctx, archivePath, metadataJSON, staging, entries); err != nil {
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

	cfg, err := opts.Database.Connection.MySQLConfig()
	if err != nil {
		return nil, err
	}

	if _, err := shop.DumpDatabase(ctx, cfg, opts.Database.Config, dumpOpts); err != nil {
		return nil, err
	}

	return databaseSource(ctx, opts.Database.Connection)
}

// databaseSource reads vendor and version of the database server.
func databaseSource(ctx context.Context, dbConn *executor.DatabaseConnection) (*DatabaseSource, error) {
	conn, cleanup, err := dbConn.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	var version string
	if err := conn.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); err != nil {
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

func exportFiles(ctx context.Context, env Environment, fc fileComponent, roots FileRoots, output string) error {
	root, err := roots.root(fc)
	if err != nil {
		return err
	}

	file, err := os.Create(output)
	if err != nil {
		return err
	}

	err = packFiles(ctx, env, root, fc.dirs, file)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("cannot export %s files: %w", fc.name, err)
	}

	return nil
}

// writeArchive writes the outer tar.gz with metadata.json first, so readers
// learn about the archive before its payloads. Only the owner may read it, as
// it can contain credentials, customer data, and private documents.
func writeArchive(ctx context.Context, path string, metadata []byte, dir string, entries []string) (err error) {
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
		if err := addFile(ctx, tw, filepath.Join(dir, entry), entry, modTime); err != nil {
			return err
		}
	}

	return nil
}

func addFile(ctx context.Context, tw *tar.Writer, path, name string, modTime time.Time) error {
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

	_, err = io.Copy(tw, archiver.ContextReader(ctx, file))
	return err
}
