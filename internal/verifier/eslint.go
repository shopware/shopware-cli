package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/shopware/shopware-cli/internal/validation"
)

// eslintOutput describes ESLint's JSON report, which uses absolute file paths.
type eslintOutput []struct {
	FilePath string          `json:"filePath"`
	Output   *string         `json:"output"`
	Messages []eslintMessage `json:"messages"`
}

type eslintMessage struct {
	RuleID   string `json:"ruleId"`
	Severity int    `json:"severity"`
	Message  string `json:"message"`
	Line     int    `json:"line"`
}

// eslintReport keeps valid findings even when other entries are malformed.
// Changed paths describe native output fields, not remaining findings.
type eslintReport struct {
	findings     []validation.CheckResult
	changedFiles []string
	errors       []error
}

type Eslint struct{}

func (e Eslint) Name() string {
	return "eslint"
}

func (e Eslint) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	return e.run(ctx, config, execution, false)
}

func (e Eslint) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary {
	if dryRun {
		return e.Check(ctx, config, execution)
	}
	return e.run(ctx, config, execution, true)
}

func (e Eslint) run(
	ctx context.Context,
	config ToolConfig,
	execution *ToolExecution,
	fix bool,
) ToolSummary {
	paths := append(append([]string{}, config.StorefrontDirectories...), config.AdminDirectories...)
	if len(paths) == 0 {
		return ToolSummary{SkipReason: "no administration or storefront sources"}
	}

	var summaryMu sync.Mutex
	changedFiles := make(map[string]struct{})
	failed := false

	runDirectories(ctx, paths, func(directory string) {
		report := e.runDirectory(ctx, config, execution, directory, fix)
		if !fix {
			return
		}

		summaryMu.Lock()
		defer summaryMu.Unlock()
		failed = failed || len(report.errors) > 0
		for _, file := range report.changedFiles {
			changedFiles[file] = struct{}{}
		}
	})

	if !fix || failed {
		return ToolSummary{}
	}

	return ToolSummary{Note: fileChangeNote(len(changedFiles), "fixed")}
}

func (e Eslint) runDirectory(
	ctx context.Context,
	config ToolConfig,
	execution *ToolExecution,
	directory string,
	fix bool,
) eslintReport {
	args := []string{
		path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "eslint"),
		"--config", path.Join(
			config.ToolDirectory, "js", "configs",
			fmt.Sprintf("eslint.config.%s.mjs", path.Base(directory)),
		),
		"--ignore-pattern", "dist/**",
		"--ignore-pattern", ".tmp/**",
		"--ignore-pattern", "vendor/**",
		"--ignore-pattern", "test/e2e/**",
		"--ignore-pattern", "**/jest.config.js",
		"--no-error-on-unmatched-pattern",
		"--format=json",
	}

	operation := OperationCheck
	if fix {
		operation = OperationFix
		args = append(args, "--fix")
	}

	cmd := CommandContext(ctx, "node", args...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "SHOPWARE_VERSION="+config.MinShopwareVersion)
	subprocess := execution.Run(operation, cmd)

	report := parseESLintReport(subprocess, config.RootDir)
	execution.AddFindings(report.findings...)
	for _, err := range report.errors {
		execution.Fail(subprocess.ToolError(err))
	}

	return report
}

func parseESLintReport(subprocess SubprocessResult, root string) eslintReport {
	if !expectedExit(subprocess.Err, 1) {
		return eslintReport{errors: []error{
			fmt.Errorf("cannot run eslint in %s: %w", subprocess.Dir, subprocess.Err),
		}}
	}

	var output eslintOutput
	if err := json.Unmarshal([]byte(subprocess.Stdout), &output); err != nil {
		return eslintReport{errors: []error{
			fmt.Errorf("cannot parse eslint output in %s: %w", subprocess.Dir, err),
		}}
	}
	if output == nil {
		return eslintReport{errors: []error{
			errors.New("cannot parse eslint output: expected a report array"),
		}}
	}

	var report eslintReport
	for _, file := range output {
		if file.FilePath == "" || file.Messages == nil {
			report.errors = append(report.errors,
				errors.New("cannot parse eslint output: incomplete file result"),
			)
			continue
		}
		if len(file.Messages) == 0 && file.Output == nil {
			continue
		}

		filePath := validation.NormalizeSourcePath(file.FilePath, root)
		if file.Output != nil {
			report.changedFiles = append(report.changedFiles, filePath)
		}

		for _, message := range file.Messages {
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
			errors.New("cannot complete eslint analysis: unsuccessful exit without diagnostics"),
		)
	}

	return report
}

func (m eslintMessage) finding(filePath string) (validation.CheckResult, error) {
	if (m.Severity != 1 && m.Severity != 2) || strings.TrimSpace(m.Message) == "" {
		return validation.CheckResult{}, errors.New(
			"cannot parse eslint output: incomplete or invalid diagnostic",
		)
	}

	severity := validation.SeverityWarning
	if m.Severity == 2 {
		severity = validation.SeverityError
	}

	rule := m.RuleID
	if rule == "" {
		rule = "diagnostic"
	}

	return validation.CheckResult{
		Path:       filePath,
		Line:       m.Line,
		Message:    m.Message,
		Severity:   severity,
		Identifier: "eslint/" + rule,
	}, nil
}

func init() {
	AddTool(Eslint{})
}
