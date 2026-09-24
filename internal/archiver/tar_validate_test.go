package archiver

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func validationArchive(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tw := tar.NewWriter(gz)
	for _, header := range headers {
		require.NoError(t, tw.WriteHeader(header))
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return out.Bytes()
}

func TestValidateTarGz(t *testing.T) {
	file := func(name string) *tar.Header { return &tar.Header{Name: name, Mode: 0o644, Typeflag: tar.TypeReg} }
	link := func(name, target string) *tar.Header {
		return &tar.Header{Name: name, Mode: 0o777, Typeflag: tar.TypeSymlink, Linkname: target}
	}
	for _, tc := range []struct {
		name    string
		headers []*tar.Header
		wantErr string
	}{
		{"regular", []*tar.Header{file("composer.json")}, ""},
		{"relative symlinks", []*tar.Header{file("dir/file"), link("dir/link", "file"), link("link", "dir/link")}, ""},
		{"parent target", []*tar.Header{file("file"), link("dir/link", "../file")}, ""},
		{"implicit directory", []*tar.Header{file("dir/file"), link("link", "dir")}, ""},
		{"absolute file", []*tar.Header{file("/outside")}, "unsafe archive path"},
		{"escaping file", []*tar.Header{file("../outside")}, "unsafe archive path"},
		{"unclean path", []*tar.Header{file("dir/../file")}, "unsafe archive path"},
		{"duplicate", []*tar.Header{file("file"), file("./file")}, "duplicate archive path"},
		{"absolute link", []*tar.Header{link("link", "/outside")}, "relative target"},
		{"escaping link", []*tar.Header{link("link", "../outside")}, "outside the project"},
		{"missing target", []*tar.Header{link("link", "missing")}, "missing target"},
		{"cyclic links", []*tar.Header{link("a", "b"), link("b", "a")}, "cycle"},
		{"symlink parent", []*tar.Header{link("dir", "."), file("dir/file")}, "used as a directory"},
		{"file parent", []*tar.Header{file("dir"), file("dir/file")}, "used as a directory"},
		{"symlink before dotdot", []*tar.Header{file("outside"), link("dir", "."), link("link", "dir/../outside")}, "outside the project"},
		{"hard link", []*tar.Header{{Name: "link", Typeflag: tar.TypeLink, Linkname: "file"}}, "unsupported archive entry"},
		{"device", []*tar.Header{{Name: "device", Typeflag: tar.TypeChar}}, "unsupported archive entry"},
		{"setuid", []*tar.Header{{Name: "file", Typeflag: tar.TypeReg, Mode: 0o4755}}, "special permission"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := validationArchive(t, tc.headers...)
			err := ValidateTarGz(t.Context(), bytes.NewReader(data))
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateTarGzChecksCompressionAndCancellation(t *testing.T) {
	data := validationArchive(t, &tar.Header{Name: "file", Typeflag: tar.TypeReg})
	data[len(data)-8] ^= 0xff // Corrupt the gzip checksum, after the tar EOF.
	require.ErrorIs(t, ValidateTarGz(t.Context(), bytes.NewReader(data)), gzip.ErrChecksum)
	require.Error(t, ValidateTarGz(t.Context(), bytes.NewReader(data[:len(data)-12])))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ValidateTarGz(ctx, bytes.NewReader(data)), context.Canceled)
}
