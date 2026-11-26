// Package projectarchive writes and reads portable shop-data archives holding
// the database and the files of the public and private Shopware filesystems.
// The layout is described in docs/adr/2026-08-11-project-import-export.md.
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
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/executor"
)

// FormatVersion is the metadata.json format_version written and accepted.
const FormatVersion = 1

// Fixed entry names of the archive layout.
const (
	metadataEntry     = "metadata.json"
	databaseEntry     = "database.sql"
	databaseDirEntry  = "database/"
	publicFilesEntry  = "public.tar.gz"
	privateFilesEntry = "private.tar.gz"
)

// Environment runs commands in the Shopware project an archive is exported
// from or imported into.
type Environment interface {
	ConsoleCommand(ctx context.Context, args ...string) *executor.Process
	Command(ctx context.Context, name string, args ...string) *executor.Process
}

// Components selects the parts of an archive.
type Components struct {
	Database     bool
	PublicFiles  bool
	PrivateFiles bool
}

// Any reports whether at least one component is selected.
func (c Components) Any() bool {
	return c.Database || c.PublicFiles || c.PrivateFiles
}

// FilesToEmpty returns the file components whose directories an import of
// these components empties, as an imported database without files has no rows
// for them.
func (c Components) FilesToEmpty() Components {
	return Components{
		PublicFiles:  c.Database && !c.PublicFiles,
		PrivateFiles: c.Database && !c.PrivateFiles,
	}
}

// FilesReplaced returns the file components an import of these components
// replaces: the ones it contains and the ones it empties.
func (c Components) FilesReplaced() Components {
	toEmpty := c.FilesToEmpty()

	return Components{
		PublicFiles:  c.PublicFiles || toEmpty.PublicFiles,
		PrivateFiles: c.PrivateFiles || toEmpty.PrivateFiles,
	}
}

// Metadata is the content of metadata.json.
type Metadata struct {
	FormatVersion int       `json:"format_version"`
	CreatedAt     time.Time `json:"created_at"`
	Source        Source    `json:"source"`
}

// Source describes the shop an archive was exported from.
type Source struct {
	ShopwareVersion string          `json:"shopware_version"`
	Database        *DatabaseSource `json:"database,omitempty"`
}

// DatabaseSource describes the database server an archive was exported from.
type DatabaseSource struct {
	Vendor  string `json:"vendor"`
	Version string `json:"version"`
}

// Contents describes an archive: its metadata and the components it holds.
type Contents struct {
	Metadata Metadata
	Components
}

// Inspect reads the metadata and detects the components of an archive
// without applying anything.
func Inspect(path string) (*Contents, error) {
	var contents Contents
	var hasMetadata, hasDatabaseDir bool

	err := walkArchive(path, func(name string, r io.Reader) error {
		switch {
		case name == metadataEntry:
			if err := json.NewDecoder(r).Decode(&contents.Metadata); err != nil {
				return fmt.Errorf("cannot parse %s: %w", metadataEntry, err)
			}
			hasMetadata = true
		case name == databaseEntry:
			contents.Database = true
		case strings.HasPrefix(name, databaseDirEntry):
			hasDatabaseDir = true
		case name == publicFilesEntry:
			contents.PublicFiles = true
		case name == privateFilesEntry:
			contents.PrivateFiles = true
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot read archive %s: %w", path, err)
	}

	switch {
	case !hasMetadata:
		return nil, fmt.Errorf("cannot read archive %s: %s is missing", path, metadataEntry)
	case contents.Metadata.FormatVersion != FormatVersion:
		return nil, fmt.Errorf("cannot read archive %s: unsupported format version %d", path, contents.Metadata.FormatVersion)
	case contents.Database && hasDatabaseDir:
		return nil, fmt.Errorf("cannot read archive %s: it contains both %s and %s", path, databaseEntry, databaseDirEntry)
	case hasDatabaseDir:
		return nil, fmt.Errorf("cannot read archive %s: MySQL Shell dumps in %s are not supported yet", path, databaseDirEntry)
	case !contents.Any():
		return nil, fmt.Errorf("cannot read archive %s: it contains neither a database nor files", path)
	}

	return &contents, nil
}

// walkArchive calls fn for every regular file of the outer tar.gz archive.
// Names are normalized without "./".
func walkArchive(path string, fn func(name string, r io.Reader) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		name := strings.TrimPrefix(header.Name, "./")
		if header.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/") + "/"
		} else if header.Typeflag != tar.TypeReg {
			continue
		}

		if err := fn(name, tr); err != nil {
			return err
		}
	}
}

// SameShopwareMinor reports whether two Shopware versions share their major
// and minor version, e.g. 6.7.1 for 6.7.1.0 and 6.7.1.2. Database migrations
// differ between minor versions.
func SameShopwareMinor(a, b string) bool {
	return shopwareMinor(a) == shopwareMinor(b)
}

func shopwareMinor(version string) string {
	parts := strings.SplitN(version, ".", 4)

	return strings.Join(parts[:min(len(parts), 3)], ".")
}
