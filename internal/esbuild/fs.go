package esbuild

import (
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

func copyStaticFiles(currentPath string, targetPath string) error {
	// When the currentPath folder does not exist, return
	if _, err := os.Stat(currentPath); os.IsNotExist(err) {
		return nil
	}

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(targetPath, os.ModePerm); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	// Walk through the current directory
	return filepath.Walk(currentPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("failed to access path %q: %w", path, err)
		}

		// Get the relative path
		relPath, err := filepath.Rel(currentPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q: %w", path, err)
		}

		// Construct target path
		targetFilePath := filepath.Join(targetPath, relPath)

		// If it's a directory, create it in target
		if info.IsDir() {
			return os.MkdirAll(targetFilePath, os.ModePerm)
		}

		// Copy the file
		return copyFile(path, targetFilePath)
	})
}

func copyFile(src, dst string) error {
	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			log.Printf("Cannot close source file: %v", err)
		}
	}()

	// Create target file
	targetFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer func() {
		if err := targetFile.Close(); err != nil {
			log.Printf("Cannot close target file: %v", err)
		}
	}()

	// Copy the contents
	if _, err := io.Copy(targetFile, sourceFile); err != nil {
		return fmt.Errorf("failed to copy file contents: %w", err)
	}

	// Copy file permissions
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to get source file info: %w", err)
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
