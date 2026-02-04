package extension

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zeebo/xxh3"

	"github.com/shopware/shopware-cli/logging"
)

// ChecksumFile generates a XXH128 checksum for a given file
func ChecksumFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("open file for checksum: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Read the file content
	data, err := io.ReadAll(file)
	if err != nil {
		return "", fmt.Errorf("read file for checksum: %w", err)
	}

	// Calculate XXH128 hash
	hash := xxh3.Hash128(data)

	// Convert the [16]byte to []byte for hex encoding
	hashBytes := hash.Bytes()
	slicedHashBytes := hashBytes[:]

	// Convert to hex string
	return hex.EncodeToString(slicedHashBytes), nil
}

type ChecksumJSON struct {
	Algorithm        string            `json:"algorithm"`
	Hashes           map[string]string `json:"hashes"`
	Version          string            `json:"version"`
	ExtensionVersion string            `json:"extensionVersion"`
}

// GenerateChecksumJSON creates a checksum.json file in the given folder
func GenerateChecksumJSON(ctx context.Context, baseFolder string, ext Extension) error {
	version, err := ext.GetVersion()
	if err != nil {
		logging.FromContext(ctx).Info("Could not determine extension version skipping checksum.json generation: ", err)

		return nil
	}

	ignores := ext.GetExtensionConfig().Build.Zip.Checksum.Ignore

	checksumData := ChecksumJSON{
		Algorithm:        "xxh128",
		Hashes:           make(map[string]string),
		Version:          "1.0.0",
		ExtensionVersion: version.String(),
	}

	// Walk through all files in the folder and calculate checksums
	err = filepath.Walk(baseFolder, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and hidden files
		if info.IsDir() {
			// Skip vendor and node_modules directories
			if info.Name() == "vendor" || info.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.Contains(path, "Resources/public/administration") {
			// Skip files in Resources/public/administration
			return nil
		}

		// Get relative path for the file
		relPath, err := filepath.Rel(baseFolder, path)
		if err != nil {
			return fmt.Errorf("get relative path: %w", err)
		}

		if slices.Contains(ignores, relPath) {
			return nil
		}

		// Skip checksum.json itself if it exists
		if relPath == "checksum.json" {
			return nil
		}

		// Skip vendor and node_modules files
		if strings.Contains(relPath, "vendor/") || strings.Contains(relPath, "node_modules/") {
			return nil
		}

		// Calculate checksum
		checksum, err := ChecksumFile(path)
		if err != nil {
			return err
		}

		// Normalize path separators to forward slashes for consistent output
		relPath = filepath.ToSlash(relPath)

		// Add to hashes map
		checksumData.Hashes[relPath] = checksum

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking directory for checksums: %w", err)
	}

	// Write checksum.json file
	checksumJSON, err := json.Marshal(checksumData)
	if err != nil {
		return fmt.Errorf("marshal checksum data: %w", err)
	}

	checksumPath := filepath.Join(baseFolder, "checksum.json")
	if err := os.WriteFile(checksumPath, checksumJSON, 0644); err != nil {
		return fmt.Errorf("write checksum file: %w", err)
	}

	return nil
}
