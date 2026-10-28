package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync/atomic"

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

func (p PHPCSFixer) Format(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary {
	if result, applicable := preparePHPTool(config, execution); !applicable {
		return result
	}
	var changed atomic.Int64
	runDirectories(config.SourceDirectories, func(directory string) {
		args := []string{
			"fix", "--show-progress", "dots", "-vv", "--no-ansi", "--format", "json",
			"--config", p.getConfigPath(config.ToolDirectory, config.RootDir), directory,
		}
		operation := OperationFormat
		if dryRun {
			operation = OperationCheck
			args = append(args, "--dry-run")
		}
		cmd := CommandContext(ctx, path.Join(config.ToolDirectory, "php", "vendor", "bin", "php-cs-fixer"), args...)
		cmd.Dir = config.RootDir
		subprocess := execution.Run(operation, cmd)
		output, stderr, runErr := []byte(subprocess.Stdout), []byte(subprocess.Stderr), subprocess.Err
		if !p.isExpectedPHPCSFixerExit(runErr) {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run php-cs-fixer in %s: %w", directory, runErr)))
			return
		}
		if diagnostics := phpCsFixerDiagnostics(string(stderr), runErr); diagnostics != "" {
			execution.AddFindings(validation.CheckResult{
				Path: directory, Message: diagnostics, Severity: validation.SeverityError, Identifier: "php-cs-fixer/error",
			})
		}
		var parsed PhpCsFixerOutput
		if err := json.Unmarshal(output, &parsed); err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse php-cs-fixer output: %w", err)))
			return
		}
		if parsed.Files == nil {
			execution.Fail(subprocess.ToolError(errors.New("cannot parse php-cs-fixer output: missing file results")))
			return
		}
		changed.Add(int64(len(parsed.Files)))
		if !dryRun {
			return
		}
		for _, file := range parsed.Files {
			for _, fixer := range file.AppliedFixers {
				execution.AddFindings(validation.CheckResult{
					Path: validation.NormalizeSourcePath(file.Name, config.RootDir), Line: 1,
					Message:  "Formatting violations found. Run format without dry-run to resolve them.",
					Severity: validation.SeverityError, Identifier: "php-cs-fixer/" + fixer,
				})
			}
		}
	})
	if !dryRun {
		return ToolSummary{Note: fileChangeNote(int(changed.Load()), "formatted")}
	}
	return ToolSummary{}
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
