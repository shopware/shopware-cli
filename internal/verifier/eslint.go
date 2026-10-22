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
	FilePath string `json:"filePath"`
	Messages []struct {
		RuleID   string `json:"ruleId"`
		Severity int    `json:"severity"`
		Message  string `json:"message"`
		Line     int    `json:"line"`
	} `json:"messages"`
}

type Eslint struct{}

func (e Eslint) Name() string {
	return "eslint"
}

func (e Eslint) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolResult {
	return e.run(ctx, config, execution, false)
}
func (e Eslint) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolResult {
	return e.run(ctx, config, execution, true)
}

func (e Eslint) run(ctx context.Context, config ToolConfig, execution *ToolExecution, fix bool) ToolResult {
	paths := append(append([]string{}, config.StorefrontDirectories...), config.AdminDirectories...)
	if len(paths) == 0 {
		return ToolResult{SkipReason: "no administration or storefront sources"}
	}
	runDirectories(paths, func(directory string) {
		args := []string{
			path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "eslint"),
			"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("eslint.config.%s.mjs", path.Base(directory))),
			"--ignore-pattern", "dist/**", "--ignore-pattern", ".tmp/**",
			"--ignore-pattern", "vendor/**", "--ignore-pattern", "test/e2e/**",
			"--ignore-pattern", "**/jest.config.js", "--no-error-on-unmatched-pattern",
		}
		if fix {
			args = append(args, "--fix")
		} else {
			args = append(args, "--format=json")
		}
		cmd := CommandContext(ctx, "node", args...)
		cmd.Dir = directory
		cmd.Env = append(os.Environ(), "SHOPWARE_VERSION="+config.MinShopwareVersion)
		subprocess := execution.Run(cmd)
		runErr := subprocess.Err
		// Fix output remains unparsed until structured fixer reports are introduced.
		if fix {
			if runErr != nil {
				execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run eslint in %s: %w", directory, runErr)))
			}
			return
		}
		if !expectedExit(runErr, 1) {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run eslint in %s: %w", directory, runErr)))
			return
		}
		var output EslintOutput
		if err := json.Unmarshal([]byte(subprocess.Stdout), &output); err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse eslint output in %s: %w", directory, err)))
			return
		}
		if output == nil {
			execution.Fail(subprocess.ToolError(errors.New("cannot parse eslint output: expected a report array")))
			return
		}
		diagnostics := 0
		for _, file := range output {
			diagnostics += len(file.Messages)
			for _, message := range file.Messages {
				severity := validation.SeverityWarning
				if message.Severity == 2 {
					severity = validation.SeverityError
				}
				execution.AddFindings(validation.CheckResult{
					Path: validation.NormalizeSourcePath(file.FilePath, config.RootDir),
					Line: message.Line, Message: message.Message, Severity: severity, Identifier: "eslint/" + message.RuleID,
				})
			}
		}
		if runErr != nil && diagnostics == 0 {
			execution.Fail(subprocess.ToolError(errors.New("cannot complete eslint analysis: unsuccessful exit without diagnostics")))
		}
	})
	return ToolResult{}
}

func init() {
	AddTool(Eslint{})
}
