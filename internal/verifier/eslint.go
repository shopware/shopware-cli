package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/validation"
)

type EslintOutput []struct {
	FilePath string  `json:"filePath"`
	Output   *string `json:"output"`
	Messages []struct {
		RuleID    string `json:"ruleId"`
		Severity  int    `json:"severity"`
		Message   string `json:"message"`
		Line      int    `json:"line"`
		Column    int    `json:"column"`
		NodeType  string `json:"nodeType"`
		EndLine   int    `json:"endLine"`
		EndColumn int    `json:"endColumn"`
		Fix       struct {
			Range []int  `json:"range"`
			Text  string `json:"text"`
		} `json:"fix,omitempty"`
		MessageID string `json:"messageId,omitempty"`
	} `json:"messages"`
	SuppressedMessages  []any  `json:"suppressedMessages"`
	ErrorCount          int    `json:"errorCount"`
	FatalErrorCount     int    `json:"fatalErrorCount"`
	WarningCount        int    `json:"warningCount"`
	FixableErrorCount   int    `json:"fixableErrorCount"`
	FixableWarningCount int    `json:"fixableWarningCount"`
	Source              string `json:"source"`
	UsedDeprecatedRules []any  `json:"usedDeprecatedRules"`
}

type Eslint struct{}

func (e Eslint) Name() string {
	return "eslint"
}

func (e Eslint) Check(ctx context.Context, config ToolConfig) ToolResult {
	return e.run(ctx, config, false)
}
func (e Eslint) Fix(ctx context.Context, config ToolConfig) ToolResult {
	return e.run(ctx, config, true)
}

func (e Eslint) run(ctx context.Context, config ToolConfig, fix bool) ToolResult {
	result := ToolResult{}
	paths := append(append([]string{}, config.StorefrontDirectories...), config.AdminDirectories...)
	if len(paths) == 0 {
		return ToolResult{SkipReason: "no administration or storefront sources"}
	}
	changed := 0
	for _, directory := range paths {
		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "eslint"), "--format=json",
			"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("eslint.config.%s.mjs", path.Base(directory))),
			"--ignore-pattern", "dist/**", "--ignore-pattern", ".tmp/**",
			"--ignore-pattern", "vendor/**", "--ignore-pattern", "test/e2e/**",
			"--ignore-pattern", "**/jest.config.js", "--no-error-on-unmatched-pattern",
		}
		if fix {
			args = append(args, "--fix")
		}
		cmd := CommandContext(ctx, "node", args...)
		cmd.Dir = directory
		cmd.Env = append(os.Environ(), "SHOPWARE_VERSION="+config.MinShopwareVersion)
		stdout, _, runErr := captureCommand(cmd, &result)
		if !expectedExit(runErr, 1) {
			result.Err = errors.Join(result.Err, fmt.Errorf("cannot run eslint in %s: %w", directory, runErr))
			continue
		}
		var output EslintOutput
		if err := json.Unmarshal(stdout, &output); err != nil {
			result.Err = errors.Join(result.Err, fmt.Errorf("cannot parse eslint output in %s: %w", directory, err))
			continue
		}
		if output == nil {
			result.Err = errors.Join(result.Err, errors.New("cannot parse eslint output: expected a report array"))
			continue
		}
		diagnostics := 0
		for _, file := range output {
			diagnostics += len(file.Messages)
			if file.Output != nil {
				changed++
			}
			for _, message := range file.Messages {
				severity := validation.SeverityWarning
				if message.Severity == 2 {
					severity = validation.SeverityError
				}
				result.Findings = append(result.Findings, validation.CheckResult{
					Path: validation.NormalizeSourcePath(file.FilePath, config.RootDir),
					Line: message.Line, Message: message.Message, Severity: severity, Identifier: "eslint/" + message.RuleID,
				})
			}
		}
		if runErr != nil && diagnostics == 0 {
			result.Err = errors.Join(result.Err, errors.New("cannot complete eslint analysis: unsuccessful exit without diagnostics"))
		}
	}
	if fix && result.Err == nil {
		result.Note = fileChangeNote(changed, "fixed")
	}
	return result
}

func init() {
	AddTool(Eslint{})
}
