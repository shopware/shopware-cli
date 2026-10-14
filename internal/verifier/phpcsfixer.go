package verifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/validation"
)

// PHP-CS-Fixer combines exit-code bitflags to report multiple conditions.
// Values are documented at https://cs.symfony.com/doc/usage.html#exit-code.
const (
	phpCsFixerExitInvalidSyntax = 4
	phpCsFixerExitNeedsFixing   = 8
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
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("cannot read composer manifest: %w", err)
	}

	var gr errgroup.Group

	for _, sourceDirectory := range config.SourceDirectories {
		gr.Go(func() error {
			fixDir := sourceDirectory

			args := []string{
				"fix",
				"--show-progress", "dots",
				"-vv",
				"--no-ansi",
				"--format", "json",
				"--config", p.getConfigPath(config.ToolDirectory, config.RootDir),
				fixDir,
			}
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
				errorOutput := commandOutputOrError(stderr.String(), output, err)

				check.AddResult(validation.CheckResult{
					Path:       fixDir,
					Message:    "failed to run php-cs-fixer: " + errorOutput,
					Severity:   validation.SeverityError,
					Line:       0,
					Identifier: "php-cs-fixer/error",
				})

				return nil
			}

			// JSON contains changed files. Lint and fixing errors are reported on stderr.
			if diagnostics := phpCsFixerDiagnostics(stderr.String(), err); diagnostics != "" {
				check.AddResult(validation.CheckResult{
					Path:       fixDir,
					Message:    "failed to format PHP files: " + diagnostics,
					Severity:   validation.SeverityError,
					Identifier: "php-cs-fixer/error",
				})
			}

			var phpCsFixerOutput PhpCsFixerOutput
			if err := json.Unmarshal(output, &phpCsFixerOutput); err != nil {
				errorOutput := commandOutputOrError(stderr.String(), output, err)

				check.AddResult(validation.CheckResult{
					Path:       fixDir,
					Message:    "failed to unmarshal php-cs-fixer output: " + errorOutput,
					Severity:   validation.SeverityError,
					Line:       0,
					Identifier: "php-cs-fixer/error",
				})

				return nil
			}

			severity := validation.SeverityWarning
			if dryRun {
				severity = validation.SeverityError
			}

			for _, file := range phpCsFixerOutput.Files {
				for _, appliedFixer := range file.AppliedFixers {
					check.AddResult(validation.CheckResult{
						Path:       validation.NormalizeSourcePath(file.Name, config.RootDir),
						Line:       1,
						Message:    ruleViolationMessage,
						Severity:   severity,
						Identifier: "php-cs-fixer/" + appliedFixer,
					})
				}
			}

			return nil
		})
	}

	return gr.Wait()
}

// phpCsFixerDiagnostics extracts errors from stderr and falls back to the process
// error when the exit code reports invalid PHP without any diagnostic output.
func phpCsFixerDiagnostics(stderr string, err error) string {
	const errorHeading = "Files that were not fixed due to errors reported during "
	if index := strings.Index(stderr, errorHeading); index >= 0 {
		return strings.TrimSpace(stderr[index:])
	}

	var exitErr *exec.ExitError
	// PHP-CS-Fixer uses bit flags for exit codes. Test whether the invalid syntax flag is set.
	if errors.As(err, &exitErr) && exitErr.ExitCode()&phpCsFixerExitInvalidSyntax != 0 {
		return commandOutputOrError(stderr, nil, err)
	}

	return ""
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

	// Combine the two accepted finding flags into one mask.
	const findings = phpCsFixerExitInvalidSyntax | phpCsFixerExitNeedsFixing

	// Apply bitwise AND to the exit code using the inverted mask of accepted flags.
	// The result is zero only when the exit code contains no flags outside the accepted mask.
	return exitErr.ExitCode() & ^findings == 0
}

func init() {
	AddTool(PHPCSFixer{})
}
