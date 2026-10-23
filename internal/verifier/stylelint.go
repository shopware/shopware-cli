package verifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/validation"
)

type StylintError struct {
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Text     string `json:"text"`
}

type StylelintOutput []struct {
	Source                string         `json:"source"`
	Deprecations          []StylintError `json:"deprecations"`
	InvalidOptionWarnings []any          `json:"invalidOptionWarnings"`
	ParseErrors           []StylintError `json:"parseErrors"`
	Errored               bool           `json:"errored"`
	Warnings              []StylintError `json:"warnings"`
}

type StyleLint struct{}

func (s StyleLint) Name() string {
	return "stylelint"
}

func (s StyleLint) Check(ctx context.Context, config ToolConfig) ToolResult {
	return s.run(ctx, config, false)
}
func (s StyleLint) Fix(ctx context.Context, config ToolConfig) ToolResult {
	return s.run(ctx, config, true)
}

func (s StyleLint) run(ctx context.Context, config ToolConfig, fix bool) ToolResult {
	result := ToolResult{}
	paths := append(append([]string{}, config.StorefrontDirectories...), config.AdminDirectories...)
	ran := false
	for _, directory := range paths {
		hasSCSS, err := hasSCSSFiles(directory)
		if err != nil {
			result.Err = errors.Join(result.Err, fmt.Errorf("cannot find SCSS files: %w", err))
			continue
		}
		if !hasSCSS {
			continue
		}
		ran = true
		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "stylelint"),
			"--formatter=json", "--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("stylelint.config.%s.mjs", path.Base(directory))),
			"--ignore-pattern", "dist/**", "--ignore-pattern", ".tmp/**", "--ignore-pattern", "vendor/**", "**/*.scss",
		}
		if fix {
			args = append(args, "--fix")
		}
		cmd := CommandContext(ctx, "node", args...)
		cmd.Dir = directory
		stdout, stderr, runErr := captureCommand(cmd, &result)
		if !expectedExit(runErr, 2) {
			result.Err = errors.Join(result.Err, fmt.Errorf("cannot run stylelint in %s: %w", directory, runErr))
			continue
		}
		outputBytes := stdout
		if len(bytes.TrimSpace(outputBytes)) == 0 {
			outputBytes = stderr
		}
		var output StylelintOutput
		if err := json.Unmarshal(outputBytes, &output); err != nil {
			result.Err = errors.Join(result.Err, fmt.Errorf("cannot parse stylelint output in %s: %w", directory, err))
			continue
		}
		if output == nil {
			result.Err = errors.Join(result.Err, errors.New("cannot parse stylelint output: expected a report array"))
			continue
		}
		diagnostics := 0
		for _, file := range output {
			if len(file.InvalidOptionWarnings) > 0 {
				result.Err = errors.Join(result.Err, fmt.Errorf("cannot configure stylelint: %v", file.InvalidOptionWarnings))
			}
			messages := append(append(file.Warnings, file.Deprecations...), file.ParseErrors...)
			diagnostics += len(messages) + len(file.InvalidOptionWarnings)
			for _, message := range messages {
				severity := message.Severity
				if severity == "" {
					severity = validation.SeverityError
				}
				result.Findings = append(result.Findings, validation.CheckResult{
					Path: validation.NormalizeSourcePath(file.Source, config.RootDir),
					Line: message.Line, Message: message.Text, Severity: severity, Identifier: "stylelint/" + message.Rule,
				})
			}
		}
		if runErr != nil && diagnostics == 0 {
			result.Err = errors.Join(result.Err, errors.New("cannot complete stylelint analysis: unsuccessful exit without diagnostics"))
		}
	}
	if !ran && result.Err == nil {
		result.SkipReason = "no SCSS source files"
	}
	return result
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
