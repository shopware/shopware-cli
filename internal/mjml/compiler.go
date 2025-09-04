package mjml

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

func Compile(ctx context.Context, mjmlPath string) (string, error) {
	// Run mjml command with the file
	cmd := exec.CommandContext(ctx, "npx", "mjml", mjmlPath, "--stdout")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("mjml compilation failed for %s: %w (stderr: %s)", mjmlPath, err, stderr.String())
	}

	if stderr.Len() > 0 {
		logging.FromContext(ctx).Warnf("MJML compilation warnings for %s: %s", mjmlPath, stderr.String())
	}

	return stdout.String(), nil
}

// ProcessDirectory walks through a directory and compiles all MJML files
func ProcessDirectory(ctx context.Context, rootDir string) error {
	var processedCount int
	var errorCount int
	var skippedCount int

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Only process .mjml files
		if !strings.HasSuffix(path, ".mjml") {
			return nil
		}

		// Only process html.mjml files
		baseName := filepath.Base(path)
		if baseName != "html.mjml" {
			logging.FromContext(ctx).Debugf("Skipping non-HTML MJML file: %s", path)
			skippedCount++
			return nil
		}

		relPath, _ := filepath.Rel(rootDir, path)
		logging.FromContext(ctx).Infof("Processing MJML file: %s", relPath)

		// Always compile to check for errors
		compiled, err := Compile(ctx, path)
		if err != nil {
			errorCount++
			logging.FromContext(ctx).Errorf("Failed to compile MJML %s: %v", relPath, err)
			return nil // Continue with other files
		}

		// Determine output path
		outputPath := getOutputPath(path)

		if err := os.WriteFile(outputPath, []byte(compiled), 0644); err != nil {
			errorCount++
			logging.FromContext(ctx).Errorf("Failed to write compiled template to %s: %v", outputPath, err)
			return nil
		}

		// Remove the original MJML file
		if err := os.Remove(path); err != nil {
			errorCount++
			logging.FromContext(ctx).Errorf("Failed to remove MJML file %s: %v", path, err)
			return nil // Continue with other files
		}
		logging.FromContext(ctx).Debugf("Removed original MJML file: %s", path)

		logging.FromContext(ctx).Infof("Successfully compiled MJML to Twig: %s -> %s", relPath, outputPath)

		processedCount++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	logging.FromContext(ctx).Infof("MJML compilation complete - processed: %d, skipped: %d, errors: %d",
		processedCount, skippedCount, errorCount)

	if errorCount > 0 {
		return fmt.Errorf("compilation completed with %d errors", errorCount)
	}

	return nil
}

// getOutputPath determines the output path for a compiled MJML file
func getOutputPath(mjmlPath string) string {
	dir := filepath.Dir(mjmlPath)
	// html.mjml -> html.twig
	return filepath.Join(dir, "html.twig")
}
