package verifier

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

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

func (p Prettier) Format(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary {
	if len(config.SourceDirectories) == 0 {
		return ToolSummary{SkipReason: "no source directories"}
	}
	var changed atomic.Int64
	runDirectories(config.SourceDirectories, func(directory string) {
		ignore, err := os.CreateTemp(directory, ".shopware-cli-prettier-ignore-*")
		if err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot create prettier ignore file: %w", err)})
			return
		}
		defer func() {
			if err := os.Remove(ignore.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
				execution.Fail(&ToolError{Err: fmt.Errorf("cannot remove prettier ignore file: %w", err)})
			}
		}()
		_, writeErr := ignore.WriteString(ignoredPaths)
		if err := errors.Join(writeErr, ignore.Close()); err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot write prettier ignore file: %w", err)})
			return
		}
		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "prettier"),
			"--config", path.Join(config.ToolDirectory, "js", "configs", "prettierrc.js"),
			"--ignore-path", ignore.Name(), "--ignore-path", ".gitignore", "--ignore-path", ".prettierignore", "--no-color", ".",
		}
		operation := "format"
		if dryRun {
			operation = "check"
			args = append(args, "--list-different")
		} else {
			args = append(args, "--write")
		}
		cmd := CommandContext(ctx, "node", args...)
		cmd.Dir = directory
		subprocess := execution.Run(operation, cmd)
		output, runErr := subprocess.Stdout, subprocess.Err
		if !p.isExpectedPrettierExit(runErr, dryRun) || strings.Contains(subprocess.Stderr, "[error]") || (runErr != nil && strings.TrimSpace(output) == "") {
			if runErr == nil {
				runErr = errors.New("prettier reported an error")
			}
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run prettier in %s: %w", directory, runErr)))
			return
		}
		parsed, err := p.Parse(output)
		if err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse prettier output: %w", err)))
			return
		}
		for _, file := range parsed.Files {
			if file.Unchanged {
				continue
			}
			changed.Add(1)
			if !dryRun {
				continue
			}
			filePath := file.Path
			if !filepath.IsAbs(filePath) {
				filePath = filepath.Join(directory, filePath)
			}
			execution.AddFindings(validation.CheckResult{
				Path:     validation.NormalizeSourcePath(filePath, config.RootDir),
				Message:  "Formatting violations found. Run format without dry-run to resolve them.",
				Severity: validation.SeverityError, Identifier: "prettier/format",
			})
		}
	})
	if !dryRun {
		return ToolSummary{Note: fileChangeNote(int(changed.Load()), "formatted")}
	}
	return ToolSummary{}
}

// isExpectedPrettierExit reports whether Prettier completed with an expected exit status.
// Exit code 1 is accepted only for dry runs. Stderr is checked separately for errors.
// All other exit codes and execution errors are treated as failures.
func (p Prettier) isExpectedPrettierExit(err error, dryRun bool) bool {
	return err == nil || (dryRun && expectedExit(err, 1))
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
