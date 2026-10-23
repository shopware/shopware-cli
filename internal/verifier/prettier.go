package verifier

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

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

const ignoredPaths = `
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

func (p Prettier) Format(ctx context.Context, config ToolConfig, dryRun bool) ToolResult {
	result := ToolResult{}
	if len(config.SourceDirectories) == 0 {
		return ToolResult{SkipReason: "no source directories"}
	}
	changed := 0
	for _, directory := range config.SourceDirectories {
		func() {
			ignore, err := os.CreateTemp(directory, ".shopware-cli-prettier-ignore-*")
			if err != nil {
				result.Err = errors.Join(result.Err, fmt.Errorf("cannot create prettier ignore file: %w", err))
				return
			}
			defer func() {
				if err := os.Remove(ignore.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
					result.Err = errors.Join(result.Err, fmt.Errorf("cannot remove prettier ignore file: %w", err))
				}
			}()
			_, writeErr := ignore.WriteString(ignoredPaths)
			if err := errors.Join(writeErr, ignore.Close()); err != nil {
				result.Err = errors.Join(result.Err, fmt.Errorf("cannot write prettier ignore file: %w", err))
				return
			}
			args := []string{
				path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "prettier"),
				"--config", path.Join(config.ToolDirectory, "js", "configs", "prettierrc.js"),
				"--ignore-path", ignore.Name(), "--no-color", ".",
			}
			if dryRun {
				args = append(args, "--list-different")
			} else {
				args = append(args, "--write")
			}
			cmd := CommandContext(ctx, "node", args...)
			cmd.Dir = directory
			output, stderr, runErr := captureCommand(cmd, &result)
			if !p.isExpectedPrettierExit(runErr, dryRun) || strings.Contains(string(stderr), "[error]") || (runErr != nil && len(bytes.TrimSpace(output)) == 0) {
				if runErr == nil {
					runErr = errors.New("prettier reported an error")
				}
				result.Err = errors.Join(result.Err, fmt.Errorf("cannot run prettier in %s: %w", directory, runErr))
				return
			}
			parsed, err := p.Parse(string(output))
			if err != nil {
				result.Err = errors.Join(result.Err, fmt.Errorf("cannot parse prettier output: %w", err))
				return
			}
			for _, file := range parsed.Files {
				if file.Unchanged {
					continue
				}
				changed++
				if !dryRun {
					continue
				}
				filePath := file.Path
				if !filepath.IsAbs(filePath) {
					filePath = filepath.Join(directory, filePath)
				}
				result.Findings = append(result.Findings, validation.CheckResult{
					Path:     validation.NormalizeSourcePath(filePath, config.RootDir),
					Message:  "Formatting violations found. Run format without dry-run to resolve them.",
					Severity: validation.SeverityError, Identifier: "prettier/format",
				})
			}
		}()
	}
	if !dryRun && result.Err == nil {
		result.Note = fileChangeNote(changed, "formatted")
	}
	return result
}

// isExpectedPrettierExit reports whether Prettier completed with an expected exit status.
// Exit code 1 is accepted only for dry runs. Stderr is checked separately for errors.
// All other exit codes and execution errors are treated as failures.
func (p Prettier) isExpectedPrettierExit(err error, dryRun bool) bool {
	if err == nil {
		return true
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}

	return dryRun && exitErr.ExitCode() == 1
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
