package verifier

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/validation"
)

type PrettierOutput struct {
	Files []PrettierOutputFile
}

type PrettierOutputFile struct {
	Path string
	// Time holds the time required to process the file in milliseconds
	Time      int
	Unchanged bool
}

var ignoredPaths = `
Resources/app/administration/.tmp/**
Resources/app/storefront/dist/**
Resources/public/**
Resources/store/**
Resources/**/vendor/**
dist/**
package-lock.json
`

type Prettier struct{}

func (p Prettier) Name() string {
	return "prettier"
}

func (p Prettier) Format(ctx context.Context, check *Check, config ToolConfig, dryRun bool) error {
	var gr errgroup.Group

	for _, sourceDirectory := range config.SourceDirectories {
		if err := os.WriteFile(path.Join(sourceDirectory, ".prettierignore"), []byte(ignoredPaths), 0o644); err != nil {
			return err
		}

		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "prettier"),
			"--config",
			path.Join(config.ToolDirectory, "js", "configs", "prettierrc.js"),
			"--no-color",
			".",
		}

		ruleViolationMessage := "Formatting violations found. Run format without dry-run to resolve them."
		if !dryRun {
			args = append(args, "--write")
			ruleViolationMessage = "Formatting violations have been fixed."
		} else {
			args = append(args, "--list-different")
		}

		gr.Go(func() error {
			cmd := CommandContext(ctx, "node", args...)
			cmd.Dir = sourceDirectory

			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			output, err := cmd.Output()

			if !p.isExpectedPrettierExit(err) {
				errorOutput := stderr.String()
				if strings.TrimSpace(errorOutput) == "" {
					errorOutput = string(output)
				}

				check.AddResult(validation.CheckResult{
					Path:       sourceDirectory,
					Message:    "failed to run prettier: " + errorOutput,
					Severity:   validation.SeverityError,
					Line:       0,
					Identifier: "prettier/error",
				})

				return nil
			}

			prettierOutput, err := p.Parse(string(output))
			if err != nil {
				errorOutput := stderr.String()
				if strings.TrimSpace(errorOutput) == "" {
					errorOutput = string(output)
				}

				check.AddResult(validation.CheckResult{
					Path:       sourceDirectory,
					Message:    "failed to parse prettier output: " + errorOutput,
					Severity:   validation.SeverityError,
					Line:       0,
					Identifier: "prettier/error",
				})

				return nil
			}

			for _, file := range prettierOutput.Files {
				if file.Unchanged {
					continue
				}

				check.AddResult(validation.CheckResult{
					Path:       validation.NormalizeSourcePath(file.Path, config.RootDir),
					Message:    ruleViolationMessage,
					Severity:   validation.SeverityWarning,
					Line:       0,
					Identifier: "prettier/format",
				})
			}

			return os.Remove(path.Join(sourceDirectory, ".prettierignore"))
		})
	}

	return gr.Wait()
}

// isExpectedPrettierExit reports whether Prettier completed with an expected exit status.
// Exit code 0 indicates success, while code 1 indicates formatting violations.
// All other exit codes and execution errors are treated as failures.
func (p Prettier) isExpectedPrettierExit(err error) bool {
	if err == nil {
		return true
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	return exitErr.ExitCode() == 1
}

// Parse converts Prettier output into structured results.
// Supports both --list-different and --write output formats.
func (p Prettier) Parse(output string) (PrettierOutput, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return PrettierOutput{}, nil
	}

	var files []PrettierOutputFile
	var parsingErrors []error

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		file := PrettierOutputFile{}

		// --write marks files that did not require changes.
		if strings.HasSuffix(line, " (unchanged)") {
			file.Unchanged = true
			line = strings.TrimSuffix(line, " (unchanged)")
		}

		// --write includes processing time in milliseconds.
		if idx := strings.LastIndex(line, " "); idx >= 0 && strings.HasSuffix(line, "ms") {
			time, err := strconv.Atoi(strings.TrimSuffix(line[idx+1:], "ms"))
			if err != nil {
				parsingErrors = append(parsingErrors, fmt.Errorf(
					"failed to parse processing time for %q: %w",
					line,
					err))
				continue
			}

			file.Time = time
			line = line[:idx]
		}

		file.Path = line
		files = append(files, file)
	}

	if len(parsingErrors) > 0 {
		return PrettierOutput{}, errors.Join(parsingErrors...)
	}

	return PrettierOutput{Files: files}, nil
}

func init() {
	AddTool(Prettier{})
}
