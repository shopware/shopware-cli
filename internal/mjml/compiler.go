package mjml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

// CompileOptions controls how the underlying mjml CLI is invoked.
type CompileOptions struct {
	// When true, passes --config.allowIncludes=true to mjml so that
	// <mj-include> directives are processed. MJML 5 ignores them by default.
	AllowIncludes bool
	// Allowlist of directories that mj-include is permitted to read from, on
	// top of the directory of the file being compiled. Values are forwarded to
	// mjml verbatim as --config.includePath=[...]; relative paths are resolved
	// by mjml against its own working directory, so callers should pass
	// absolute paths. Setting any value implies AllowIncludes.
	IncludePaths []string
}

// NewCompileOptions builds CompileOptions for files compiled inside the given
// search-path root. When mj-include is in use, searchPathRoot is automatically
// allowlisted so that templates can include any partial inside the same search
// path (e.g. a sibling _includes/ folder) without having to enumerate each
// directory. extraIncludePaths are appended after the search-path root.
//
// When both allowIncludes is false and extraIncludePaths is empty, the
// returned CompileOptions is the zero value and no include flags are
// forwarded to mjml.
func NewCompileOptions(searchPathRoot string, allowIncludes bool, extraIncludePaths []string) CompileOptions {
	if !allowIncludes && len(extraIncludePaths) == 0 {
		return CompileOptions{}
	}

	paths := make([]string, 0, 1+len(extraIncludePaths))
	if searchPathRoot != "" {
		paths = append(paths, searchPathRoot)
	}
	paths = append(paths, extraIncludePaths...)

	return CompileOptions{AllowIncludes: true, IncludePaths: paths}
}

func Compile(ctx context.Context, mjmlPath string, opts CompileOptions) (string, error) {
	args := []string{"mjml", mjmlPath, "--stdout"}

	if opts.AllowIncludes || len(opts.IncludePaths) > 0 {
		args = append(args, "--config.allowIncludes=true")
	}

	if len(opts.IncludePaths) > 0 {
		encoded, err := json.Marshal(opts.IncludePaths)
		if err != nil {
			return "", fmt.Errorf("failed to encode mjml include paths: %w", err)
		}
		args = append(args, fmt.Sprintf("--config.includePath=%s", string(encoded)))
	}

	cmd := exec.CommandContext(ctx, "npx", args...)

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
func ProcessDirectory(ctx context.Context, rootDir string, opts CompileOptions) error {
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
		compiled, err := Compile(ctx, path, opts)
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
