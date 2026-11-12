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

// stylelintOutput describes native file reports, whose source paths are absolute.
type stylelintOutput []struct {
	Source                string             `json:"source"`
	Autofixed             bool               `json:"autofixed"`
	Deprecations          []stylelintMessage `json:"deprecations"`
	InvalidOptionWarnings []any              `json:"invalidOptionWarnings"`
	ParseErrors           []stylelintMessage `json:"parseErrors"`
	Warnings              []stylelintMessage `json:"warnings"`
}

type stylelintMessage struct {
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Text     string `json:"text"`
}

type StyleLint struct{}

func (s StyleLint) Name() string {
	return "stylelint"
}

func (s StyleLint) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	return s.run(ctx, config, execution, false)
}

func (s StyleLint) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary {
	if dryRun {
		return s.Check(ctx, config, execution)
	}
	return s.run(ctx, config, execution, true)
}

func (s StyleLint) run(
	ctx context.Context,
	config ToolConfig,
	execution *ToolExecution,
	fix bool,
) ToolSummary {
	paths := append(append([]string{}, config.StorefrontDirectories...), config.AdminDirectories...)
	var ran atomic.Bool
	runDirectories(ctx, paths, func(directory string) {
		hasSCSS, err := hasSCSSFiles(ctx, directory)
		if err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot find SCSS files: %w", err)})
			return
		}
		if !hasSCSS {
			return
		}
		ran.Store(true)
		s.runDirectory(ctx, config, execution, directory, fix)
	})
	if !ran.Load() {
		return ToolSummary{SkipReason: "no SCSS source files"}
	}
	if fix {
		return ToolSummary{ChangeAction: ChangeActionFixed}
	}
	return ToolSummary{}
}

func (s StyleLint) runDirectory(
	ctx context.Context,
	config ToolConfig,
	execution *ToolExecution,
	directory string,
	fix bool,
) {
	args := []string{
		path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "stylelint"),
		"--config", path.Join(
			config.ToolDirectory, "js", "configs",
			fmt.Sprintf("stylelint.config.%s.mjs", path.Base(directory)),
		),
		"--ignore-pattern", "dist/**",
		"--ignore-pattern", ".tmp/**",
		"--ignore-pattern", "vendor/**",
		"--formatter=json", "**/*.scss",
	}

	operation := OperationCheck
	if fix {
		operation = OperationFix
		args = append(args, "--fix")
	}

	cmd := CommandContext(ctx, "node", args...)
	cmd.Dir = directory
	subprocess := execution.Run(operation, cmd)

	report := parseStylelintReport(subprocess, config.RootDir)
	execution.AddFindings(report.findings...)
	if fix {
		execution.AddChangedFiles(report.changedFiles...)
	}
	for _, err := range report.errors {
		execution.Fail(subprocess.ToolError(err))
	}
}

func parseStylelintReport(subprocess SubprocessResult, root string) parsedToolReport {
	if !expectedExit(subprocess.Err, 2) {
		return parsedToolReport{errors: []error{
			fmt.Errorf("cannot run stylelint in %s: %w", subprocess.Dir, subprocess.Err),
		}}
	}

	output := subprocess.Stdout
	if strings.TrimSpace(output) == "" {
		output = subprocess.Stderr
	}
	var parsed stylelintOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return parsedToolReport{errors: []error{
			fmt.Errorf("cannot parse stylelint output in %s: %w", subprocess.Dir, err),
		}}
	}
	if parsed == nil {
		return parsedToolReport{errors: []error{
			errors.New("cannot parse stylelint output: expected a report array"),
		}}
	}

	var report parsedToolReport
	for _, file := range parsed {
		if file.Source == "" || file.Warnings == nil {
			report.errors = append(report.errors,
				errors.New("cannot parse stylelint output: incomplete file result"),
			)
			continue
		}
		if len(file.InvalidOptionWarnings) > 0 {
			report.errors = append(report.errors,
				fmt.Errorf("cannot configure stylelint: %v", file.InvalidOptionWarnings),
			)
		}

		messages := slices.Concat(file.Warnings, file.ParseErrors)
		for _, deprecation := range file.Deprecations {
			deprecation.Severity = validation.SeverityWarning
			messages = append(messages, deprecation)
		}
		if !file.Autofixed && len(messages) == 0 {
			continue
		}

		filePath := validation.NormalizeSourcePath(file.Source, root)
		if file.Autofixed {
			report.changedFiles = append(report.changedFiles, filePath)
		}

		for _, message := range messages {
			finding, err := message.finding(filePath)
			if err != nil {
				report.errors = append(report.errors, err)
				continue
			}
			report.findings = append(report.findings, finding)
		}
	}

	if subprocess.Err != nil && len(report.findings) == 0 && len(report.errors) == 0 {
		report.errors = append(report.errors,
			errors.New("cannot complete stylelint analysis: unsuccessful exit without diagnostics"),
		)
	}

	return report
}

func (m stylelintMessage) finding(filePath string) (validation.CheckResult, error) {
	severity := m.Severity
	if severity == "" {
		severity = validation.SeverityError
	}
	if (severity != validation.SeverityError && severity != validation.SeverityWarning) ||
		strings.TrimSpace(m.Text) == "" {
		return validation.CheckResult{}, errors.New(
			"cannot parse stylelint output: incomplete or invalid diagnostic",
		)
	}

	rule := m.Rule
	if rule == "" {
		rule = "diagnostic"
	}

	return validation.CheckResult{
		Path:       filePath,
		Line:       m.Line,
		Message:    m.Text,
		Severity:   severity,
		Identifier: "stylelint/" + rule,
	}, nil
}

func init() {
	AddTool(StyleLint{})
}

func hasSCSSFiles(ctx context.Context, dir string) (bool, error) {
	var found bool
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
