package mjml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

// CompilerMode defines the compilation method
type CompilerMode string

const (
	// CompilerModeLocal uses local npm mjml package
	CompilerModeLocal CompilerMode = "local"
	// CompilerModeWebService uses remote MJML API service
	CompilerModeWebService CompilerMode = "webservice"
)

// Config defines the compiler configuration
type Config struct {
	Mode             CompilerMode
	WebServiceURL    string
	WebServiceAPIKey string
}

// Compiler provides MJML to HTML compilation functionality
type Compiler interface {
	CompileFile(ctx context.Context, mjmlPath string) (string, error)
	ProcessDirectory(ctx context.Context, rootDir string, dryRun bool) error
}

type compiler struct {
	config Config
}

// NewCompiler creates a new MJML compiler instance
func NewCompiler() Compiler {
	return &compiler{
		config: Config{
			Mode: CompilerModeLocal,
		},
	}
}

// NewCompilerWithConfig creates a new MJML compiler instance with configuration
func NewCompilerWithConfig(config Config) Compiler {
	return &compiler{
		config: config,
	}
}

// CompileFile compiles an MJML file to HTML using configured method
func (c *compiler) CompileFile(ctx context.Context, mjmlPath string) (string, error) {
	if c.config.Mode == CompilerModeWebService {
		return c.compileWithWebService(ctx, mjmlPath)
	}
	return c.compileWithLocal(ctx, mjmlPath)
}

// compileWithLocal compiles using local npm mjml package
func (c *compiler) compileWithLocal(ctx context.Context, mjmlPath string) (string, error) {
	// Check if mjml command is available
	if err := c.checkMJMLAvailable(ctx); err != nil {
		return "", err
	}

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

// compileWithWebService compiles using remote MJML API service
func (c *compiler) compileWithWebService(ctx context.Context, mjmlPath string) (string, error) {
	// Read MJML file content
	mjmlContent, err := os.ReadFile(mjmlPath)
	if err != nil {
		return "", fmt.Errorf("failed to read MJML file %s: %w", mjmlPath, err)
	}

	// Prepare request payload
	payload := map[string]interface{}{
		"mjml": string(mjmlContent),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.config.WebServiceURL, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Support different authentication methods:
	// 1. API key as Bearer token (for some custom services)
	// 2. No authentication (for public endpoints like mjml.shyim.de)
	// 3. Basic auth embedded in URL (for official MJML API: https://user:key@api.mjml.io/v1/render)
	if c.config.WebServiceAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.config.WebServiceAPIKey)
	}

	// Send request
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to MJML webservice: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logging.FromContext(ctx).Warnf("Failed to close response body: %v", closeErr)
		}
	}()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("MJML webservice returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response map[string]interface{}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	html, ok := response["html"].(string)
	if !ok {
		return "", fmt.Errorf("unexpected response format from MJML webservice")
	}

	if errors, ok := response["errors"].([]interface{}); ok && len(errors) > 0 {
		logging.FromContext(ctx).Warnf("MJML compilation warnings for %s: %v", mjmlPath, errors)
	}

	return html, nil
}

// checkMJMLAvailable verifies that the mjml npm package is available
func (c *compiler) checkMJMLAvailable(ctx context.Context) error {
	// Skip check if using webservice
	if c.config.Mode == CompilerModeWebService {
		return nil
	}

	cmd := exec.CommandContext(ctx, "npx", "mjml", "--version")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mjml is not available. Please install it with: npm install -g mjml")
	}

	logging.FromContext(ctx).Debugf("MJML version: %s", strings.TrimSpace(stdout.String()))
	return nil
}

// ProcessDirectory walks through a directory and compiles all MJML files
func (c *compiler) ProcessDirectory(ctx context.Context, rootDir string, dryRun bool) error {
	var processedCount int
	var errorCount int
	var skippedCount int

	// First check if mjml is available
	if err := c.checkMJMLAvailable(ctx); err != nil {
		return err
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			return err
		}

		// Skip directories
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
		compiled, err := c.CompileFile(ctx, path)
		if err != nil {
			errorCount++
			logging.FromContext(ctx).Errorf("Failed to compile MJML %s: %v", relPath, err)
			return nil // Continue with other files
		}

		// Determine output path
		outputPath := c.getOutputPath(path)

		if dryRun {
			// In dry-run mode, check if output would be different
			existingContent, err := os.ReadFile(outputPath)
			switch {
			case err != nil && os.IsNotExist(err):
				logging.FromContext(ctx).Infof("Would create new file: %s", outputPath)
			case err != nil:
				logging.FromContext(ctx).Warnf("Cannot read existing file %s: %v", outputPath, err)
			case string(existingContent) != compiled:
				logging.FromContext(ctx).Infof("Would update file: %s", outputPath)
			default:
				logging.FromContext(ctx).Debugf("File %s is already up to date", outputPath)
			}

			// Check if MJML file would be removed
			if _, err := os.Stat(path); err == nil {
				logging.FromContext(ctx).Infof("Would remove MJML file: %s", path)
			}
		} else {
			// Write compiled HTML to twig file
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
		}

		processedCount++
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	logging.FromContext(ctx).Infof("MJML compilation complete - processed: %d, skipped: %d, errors: %d (dry_run: %v)",
		processedCount, skippedCount, errorCount, dryRun)

	if errorCount > 0 {
		return fmt.Errorf("compilation completed with %d errors", errorCount)
	}

	return nil
}

// getOutputPath determines the output path for a compiled MJML file
func (c *compiler) getOutputPath(mjmlPath string) string {
	dir := filepath.Dir(mjmlPath)
	// html.mjml -> html.twig
	return filepath.Join(dir, "html.twig")
}
