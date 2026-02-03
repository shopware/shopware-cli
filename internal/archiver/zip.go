package archiver

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Unzip extracts a zip archive to a destination directory.
func Unzip(r *zip.Reader, dest string) error {
	errorFormat := "unzip: %w"

	for _, f := range r.File {
		// Store filename/path for returning and using later on
		fpath := filepath.Join(dest, f.Name) //nolint:gosec

		// Check for ZipSlip. More Info: http://bit.ly/2MsjAWE
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("Unzip: %s: illegal file path", fpath)
		}

		if f.FileInfo().IsDir() {
			// Make Folder
			_ = os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// Make File
		if err := os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}

		_, err = io.Copy(outFile, rc) //nolint:gosec

		// Close the file without defer to close before next iteration of loop
		_ = outFile.Close()
		_ = rc.Close()

		if err != nil {
			return fmt.Errorf(errorFormat, err)
		}

		// Restore the modified time
		if err := os.Chtimes(fpath, f.Modified, f.Modified); err != nil {
			return fmt.Errorf(errorFormat, err)
		}
	}

	return nil
}

// CreateZip creates a zip archive from a source directory.
func CreateZip(baseFolder, zipFile string) error {
	// Get a Buffer to Write To
	outFile, err := os.Create(zipFile)
	if err != nil {
		return fmt.Errorf("create zipfile: %w", err)
	}

	defer func() {
		_ = outFile.Close()
	}()

	// Create a new zip archive.
	w := zip.NewWriter(outFile)

	defer func() {
		_ = w.Close()
	}()

	return AddZipFiles(w, baseFolder, "")
}

// AddZipFiles recursively adds files from a directory to a zip archive.
func AddZipFiles(w *zip.Writer, basePath, baseInZip string) error {
	files, err := os.ReadDir(basePath)
	if err != nil {
		return fmt.Errorf("could not zip dir, basePath: %q, baseInZip: %q, %w", basePath, baseInZip, err)
	}

	for _, file := range files {
		if file.IsDir() {
			// Add files of directory recursively
			if err = AddZipFiles(w, filepath.Join(basePath, file.Name()), filepath.Join(baseInZip, file.Name())); err != nil {
				return err
			}
		} else {
			if err = addFileToZip(w, filepath.Join(basePath, file.Name()), filepath.Join(baseInZip, file.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func addFileToZip(zipWriter *zip.Writer, sourcePath string, zipPath string) error {
	zipErrorFormat := "could not zip file, sourcePath: %q, zipPath: %q, %w"

	file, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	header, err := zip.FileInfoHeader(fileInfo)
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}
	header.Name = zipPath
	header.Method = zip.Deflate

	f, err := zipWriter.CreateHeader(header)
	if err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	if _, err := io.Copy(f, file); err != nil {
		return fmt.Errorf(zipErrorFormat, sourcePath, zipPath, err)
	}

	return nil
}
