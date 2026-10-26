package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/shopware/shopware-cli/internal/validation"
)

type StylintError struct {
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Text     string `json:"text"`
}

type StylelintOutput []struct {
	Source                string         `json:"source"`
	Deprecations          []StylintError `json:"deprecations"`
	InvalidOptionWarnings []any          `json:"invalidOptionWarnings"`
	ParseErrors           []StylintError `json:"parseErrors"`
	Warnings              []StylintError `json:"warnings"`
}

type StyleLint struct{}

func (s StyleLint) Name() string {
	return "stylelint"
}

func (s StyleLint) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	return s.run(ctx, config, execution, false)
}
func (s StyleLint) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	return s.run(ctx, config, execution, true)
}

func (s StyleLint) run(ctx context.Context, config ToolConfig, execution *ToolExecution, fix bool) ToolSummary {
	paths := append(append([]string{}, config.StorefrontDirectories...), config.AdminDirectories...)
	var ran atomic.Bool
	runDirectories(paths, func(directory string) {
		hasSCSS, err := hasSCSSFiles(directory)
		if err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot find SCSS files: %w", err)})
			return
		}
		if !hasSCSS {
			return
		}
		ran.Store(true)
		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "stylelint"),
			"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("stylelint.config.%s.mjs", path.Base(directory))),
			"--ignore-pattern", "dist/**", "--ignore-pattern", ".tmp/**", "--ignore-pattern", "vendor/**", "**/*.scss",
		}
		if fix {
			args = append(args, "--fix")
		} else {
			args = append(args, "--formatter=json")
		}
		cmd := CommandContext(ctx, "node", args...)
		cmd.Dir = directory
		subprocess := execution.Run(cmd)
		runErr := subprocess.Err
		// Fix output remains unparsed until structured fixer reports are introduced.
		if fix {
			if runErr != nil {
				execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run stylelint in %s: %w", directory, runErr)))
			}
			return
		}
		if !expectedExit(runErr, 2) {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run stylelint in %s: %w", directory, runErr)))
			return
		}
		output := subprocess.Stdout
		if strings.TrimSpace(output) == "" {
			output = subprocess.Stderr
		}
		var parsed StylelintOutput
		if err := json.Unmarshal([]byte(output), &parsed); err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse stylelint output in %s: %w", directory, err)))
			return
		}
		if parsed == nil {
			execution.Fail(subprocess.ToolError(errors.New("cannot parse stylelint output: expected a report array")))
			return
		}
		diagnostics := 0
		for _, file := range parsed {
			if len(file.InvalidOptionWarnings) > 0 {
				execution.Fail(subprocess.ToolError(fmt.Errorf("cannot configure stylelint: %v", file.InvalidOptionWarnings)))
			}
			messages := slices.Concat(file.Warnings, file.Deprecations, file.ParseErrors)
			diagnostics += len(messages) + len(file.InvalidOptionWarnings)
			for _, message := range messages {
				severity := message.Severity
				if severity == "" {
					severity = validation.SeverityError
				}
				execution.AddFindings(validation.CheckResult{
					Path: validation.NormalizeSourcePath(file.Source, config.RootDir),
					Line: message.Line, Message: message.Text, Severity: severity, Identifier: "stylelint/" + message.Rule,
				})
			}
		}
		if runErr != nil && diagnostics == 0 {
			execution.Fail(subprocess.ToolError(errors.New("cannot complete stylelint analysis: unsuccessful exit without diagnostics")))
		}
	})
	if !ran.Load() {
		return ToolSummary{SkipReason: "no SCSS source files"}
	}
	return ToolSummary{}
}

func init() {
	AddTool(StyleLint{})
}

func hasSCSSFiles(dir string) (bool, error) {
	var found bool
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && (d.Name() == "node_modules" || d.Name() == "vendor" || d.Name() == "dist") {
			return filepath.SkipDir
		}

		if !d.IsDir() && strings.HasSuffix(path, ".scss") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	return found, nil
}
