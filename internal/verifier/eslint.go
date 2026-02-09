package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/validation"
)

type EslintOutput []struct {
	FilePath string `json:"filePath"`
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

func (e Eslint) Check(ctx context.Context, check *Check, config ToolConfig) error {
	paths := append([]string{}, config.StorefrontDirectories...)
	paths = append(paths, config.AdminDirectories...)

	var gr errgroup.Group

	env := append(os.Environ(), fmt.Sprintf("SHOPWARE_VERSION=%s", config.MinShopwareVersion))

	for _, p := range paths {
		p := p

		gr.Go(func() error {
			eslint := exec.CommandContext(ctx,
				"node",
				path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "eslint"),
				"--format=json",
				"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("eslint.config.%s.mjs", path.Base(p))),
				"--ignore-pattern", "dist/**",
				"--ignore-pattern", ".tmp/**",
				"--ignore-pattern", "vendor/**",
				"--ignore-pattern", "test/e2e/**",
				"--ignore-pattern", "**/jest.config.js",
				"--no-error-on-unmatched-pattern",
			)
			eslint.Dir = p
			eslint.Env = env

			log, _ := eslint.CombinedOutput()

			var eslintOutput EslintOutput

			if err := json.Unmarshal(log, &eslintOutput); err != nil {
				return fmt.Errorf("failed to unmarshal eslint output: %w, %s", err, string(log))
			}

			for _, diagnostic := range eslintOutput {
				fixedPath := strings.TrimPrefix(strings.TrimPrefix(diagnostic.FilePath, "/private"), config.RootDir+"/")

				for _, message := range diagnostic.Messages {
					severity := validation.SeverityWarning

					if message.Severity == 2 {
						severity = validation.SeverityError
					}

					check.AddResult(validation.CheckResult{
						Path:       fixedPath,
						Line:       message.Line,
						Message:    message.Message,
						Severity:   severity,
						Identifier: fmt.Sprintf("eslint/%s", message.RuleID),
					})
				}
			}

			return nil
		})
	}

	return gr.Wait()
}

func (e Eslint) Fix(ctx context.Context, config ToolConfig) error {
	paths := append([]string{}, config.StorefrontDirectories...)
	paths = append(paths, config.AdminDirectories...)
	env := append(os.Environ(), fmt.Sprintf("SHOPWARE_VERSION=%s", config.MinShopwareVersion))

	var gr errgroup.Group

	for _, p := range paths {
		p := p

		gr.Go(func() error {
			eslint := exec.CommandContext(ctx,
				"node",
				path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "eslint"),
				"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("eslint.config.%s.mjs", path.Base(p))),
				"--ignore-pattern", "dist/**",
				"--ignore-pattern", ".tmp/**",
				"--ignore-pattern", "vendor/**",
				"--ignore-pattern", "test/e2e/**",
				"--ignore-pattern", "**/jest.config.js",
				"--fix",
				"--no-error-on-unmatched-pattern",
			)
			eslint.Dir = p
			eslint.Env = env

			log, _ := eslint.CombinedOutput()

			//nolint: forbidigo
			fmt.Print(string(log))

			return nil
		})
	}

	return gr.Wait()
}

func (e Eslint) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
}

func init() {
	AddTool(Eslint{})
}
