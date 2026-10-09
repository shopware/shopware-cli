package verifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/validation"
)

type PhpCsFixerOutput struct {
	Files []struct {
		Name          string   `json:"name"`
		AppliedFixers []string `json:"appliedFixers,omitempty"`
	} `json:"files"`

	Time struct {
		Total float64 `json:"total"`
	} `json:"time"`

	Memory float64 `json:"memory"`
}

type PHPCSFixer struct{}

func (p PHPCSFixer) Name() string {
	return "php-cs-fixer"
}

func (p PHPCSFixer) getConfigPath(toolDirectory, rootDir string) string {
	if _, err := os.Stat(path.Join(rootDir, ".php-cs-fixer.dist.php")); err == nil {
		return path.Join(rootDir, ".php-cs-fixer.dist.php")
	}

	return path.Join(toolDirectory, "php", "configs", "php-cs-fixer.dist.php")
}

func (p PHPCSFixer) Format(ctx context.Context, check *Check, config ToolConfig, dryRun bool) error {
	// Apps don't have a composer.json file, skip them
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		//nolint: nilerr
		return nil
	}

	var gr errgroup.Group

	for _, sourceDirectory := range config.SourceDirectories {
		gr.Go(func() error {
			fixDir := sourceDirectory

			args := []string{"fix", "--show-progress", "dots", "--verbose", "--format", "json", "--config", p.getConfigPath(config.ToolDirectory, config.RootDir), fixDir}
			ruleViolationMessage := "Formatting violations have been fixed."
			if dryRun {
				args = append(args, "--dry-run")
				ruleViolationMessage = "Formatting violations found. Run format without dry-run to resolve them."
			}

			cmd := CommandContext(ctx, path.Join(config.ToolDirectory, "php", "vendor", "bin", "php-cs-fixer"), args...)
			cmd.Dir = config.RootDir

			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			output, err := cmd.Output()

			if !p.isExpectedPHPCSFixerExit(err) {
				errorOutput := stderr.String()
				if strings.TrimSpace(errorOutput) == "" {
					errorOutput = string(output)
				}

				check.AddResult(validation.CheckResult{
					Path:       fixDir,
					Message:    "failed to run php-cs-fixer: " + errorOutput,
					Severity:   validation.SeverityError,
					Line:       0,
					Identifier: "php-cs-fixer/error",
				})

				return nil
			}

			var phpCsFixerOutput PhpCsFixerOutput
			if err := json.Unmarshal(output, &phpCsFixerOutput); err != nil {
				errorOutput := stderr.String()
				if strings.TrimSpace(errorOutput) == "" {
					errorOutput = string(output)
				}

				check.AddResult(validation.CheckResult{
					Path:       fixDir,
					Message:    "failed to unmarshal php-cs-fixer output: " + errorOutput,
					Severity:   validation.SeverityError,
					Line:       0,
					Identifier: "php-cs-fixer/error",
				})

				return nil
			}

			for _, file := range phpCsFixerOutput.Files {
				for _, appliedFixer := range file.AppliedFixers {
					check.AddResult(validation.CheckResult{
						Path:       validation.NormalizeSourcePath(file.Name, config.RootDir),
						Line:       1,
						Message:    ruleViolationMessage,
						Severity:   validation.SeverityWarning,
						Identifier: "php-cs-fixer/" + appliedFixer,
					})
				}
			}

			return nil
		})
	}

	return gr.Wait()
}

// isExpectedPHPCSFixerExit reports whether PHP-CS-Fixer completed without a critical error.
// A nil error indicates a successful exit (code 0). Non-zero exit codes are accepted
// only if they contain known reporting flags for invalid syntax or files needing fixes.
func (p PHPCSFixer) isExpectedPHPCSFixerExit(err error) bool {
	if err == nil {
		return true
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	const (
		invalidSyntax = 4
		needsFixing   = 8
	)

	const findings = invalidSyntax | needsFixing

	return exitErr.ExitCode() & ^findings == 0
}

func init() {
	AddTool(PHPCSFixer{})
}
