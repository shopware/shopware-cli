package system

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

func CopyFiles(currentPath string, targetPath string) error {
	// When the currentPath folder does not exist, return
	if _, err := os.Stat(currentPath); os.IsNotExist(err) {
		return nil
	}

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
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

		// Skip development environment and VCS metadata folders
		// (e.g., .devenv, .direnv, .git)
		if info.IsDir() && (relPath == ".devenv" || relPath == ".direnv" || relPath == ".git") {
			return filepath.SkipDir
		}

		// Construct target path
		targetFilePath := filepath.Join(targetPath, relPath)

		// If it's a directory, create it in target
		if info.IsDir() {
			return os.MkdirAll(targetFilePath, 0o755)
		}

		// Copy the file
		return copyFile(path, targetFilePath)
	})
}

func copyFile(src, dst string) error {
	// Check if it's a symlink
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	// If it's a symlink, create a new symlink
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(src)
		if err != nil {
			return fmt.Errorf("failed to read symlink: %w", err)
		}
		return os.Symlink(linkTarget, dst)
	}

	// Open source file
	sourceFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer func() {
		if closeErr := sourceFile.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close source file: %w", closeErr)
		}
	}()

	// Create target file
	targetFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create target file: %w", err)
	}
	defer func() {
		if closeErr := targetFile.Close(); closeErr != nil {
			err = fmt.Errorf("failed to close target file: %w", closeErr)
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

func IsDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close()
	}()

	_, err = f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err
}
