package esbuild

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/compress/zip"
)

func downloadDartSass(ctx context.Context, cacheDir string) error {
	request, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://github.com/sass/dart-sass/releases/download/%s/dart-sass-%s-windows-x64.zip", dartSassVersion, dartSassVersion), nil)

	zipFile, err := http.DefaultClient.Do(request)

	if err != nil {
		return fmt.Errorf("cannot download dart-sass: %w", err)
	}

	defer zipFile.Body.Close()

	if zipFile.StatusCode != 200 {
		return fmt.Errorf("cannot download dart-sass: %s with http code %s", zipFile.Request.URL, zipFile.Status)
	}

	zipBytes, err := io.ReadAll(zipFile.Body)

	if err != nil {
		return fmt.Errorf("cannot read zip file: %w", err)
	}

	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))

	if err != nil {
		return fmt.Errorf("cannot read zip file: %w", err)
	}

	for _, zipFile := range zipReader.File {
		name := strings.TrimPrefix(zipFile.Name, "dart-sass/")

		if strings.Contains(name, "..") {
			continue
		}

		zipHandle, err := zipFile.Open()

		if err != nil {
			return fmt.Errorf("cannot open zip file: %w", err)
		}

		extractedPath := filepath.Join(cacheDir, name)
		extractedFolder := filepath.Dir(extractedPath)

		if _, err := os.Stat(extractedFolder); os.IsNotExist(err) {
			if err := os.MkdirAll(extractedFolder, os.ModePerm); err != nil {
				return fmt.Errorf("cannot create dart-sass in temp: %w", err)
			}
		}

		outFile, err := os.Create(extractedPath)

		if err != nil {
			return fmt.Errorf("cannot create dart-sass in temp: %w", err)
		}

		if _, err := io.CopyN(outFile, zipHandle, int64(zipFile.UncompressedSize64)); err != nil {
			return fmt.Errorf("cannot copy dart-sass in temp: %w", err)
		}

		if err := outFile.Close(); err != nil {
			return fmt.Errorf("cannot close dart-sass in temp: %w", err)
		}
	}

	return nil
}
