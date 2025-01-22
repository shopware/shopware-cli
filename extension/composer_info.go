package extension

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"io"

	"github.com/klauspost/compress/zstd"
)

//go:embed composer-info.zip.zst
var composerInfoFile []byte

func getComposerInfoFS() (*zip.Reader, error) {
	zstReader, err := zstd.NewReader(bytes.NewReader(composerInfoFile))
	if err != nil {
		return nil, err
	}

	defer zstReader.Close()

	uncompressed, err := io.ReadAll(zstReader)
	if err != nil {
		return nil, err
	}

	return zip.NewReader(bytes.NewReader(uncompressed), int64(len(uncompressed)))
}
