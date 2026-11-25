// Package projectarchive writes and reads portable shop-data archives holding
// the database and the files of the public and private Shopware filesystems.
// The layout is described in docs/adr/2026-08-11-project-import-export.md.
package projectarchive

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

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

// walkArchive calls fn for every regular file of the outer archive, which may
// be a plain, gzip or zstd compressed tar. Names are normalized without "./".
func walkArchive(path string, fn func(name string, r io.Reader) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	r, closeReader, err := decompress(bufio.NewReader(file))
	if err != nil {
		return err
	}
	defer closeReader()

	tr := tar.NewReader(r)
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

var (
	gzipMagic = []byte{0x1f, 0x8b}
	zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}
)

// decompress detects the compression of the outer archive by its magic bytes.
func decompress(r *bufio.Reader) (io.Reader, func(), error) {
	magic, err := r.Peek(len(zstdMagic))
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}

	switch {
	case bytes.HasPrefix(magic, gzipMagic):
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return gz, func() { _ = gz.Close() }, nil
	case bytes.HasPrefix(magic, zstdMagic):
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, nil, err
		}
		return zr, zr.Close, nil
	default:
		return r, func() {}, nil
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
