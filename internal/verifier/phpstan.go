package verifier

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

var possiblePHPStanConfigs = []string{
	"phpstan.neon",
	"phpstan.neon.dist",
	"phpstan.dist.neon",
}

type PhpStanOutput struct {
	Files map[string]struct {
		Messages []struct {
			Message    string `json:"message"`
			Line       int    `json:"line"`
			Identifier string `json:"identifier"`
			Tip        string `json:"tip"`
		} `json:"messages"`
	} `json:"files"`
	Errors []string `json:"errors"`
}

type PhpStan struct{}

func (p PhpStan) Name() string {
	return "phpstan"
}

func (p PhpStan) configExists(pluginPath string) bool {
	for _, config := range possiblePHPStanConfigs {
		if _, err := os.Stat(path.Join(pluginPath, config)); err == nil {
			return true
		}
	}

	return false
}

func (p PhpStan) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	if result, applicable := preparePHPTool(config, execution); !applicable {
		return result
	}
	note := ""
	_, vendorErr := os.Stat(path.Join(config.RootDir, "vendor"))
	if err := installComposerDeps(ctx, config.RootDir, config.CheckAgainst, execution); err != nil {
		execution.Fail(err)
		return ToolSummary{}
	}
	if installed := shop.DetectShopwareVersion(config.RootDir); installed != "" {
		note = "Shopware " + installed
		if errors.Is(vendorErr, os.ErrNotExist) && config.CheckAgainst != "" {
			note += " (" + config.CheckAgainst + ")"
		}
	}
	analysed := false
	for _, directory := range config.SourceDirectories {
		args := []string{"-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "phpstan"), "analyse", "--no-progress", "--no-interaction", "--error-format=json", directory}
		if !p.configExists(config.RootDir) {
			args = append(args, "--configuration", path.Join(config.ToolDirectory, "php", "configs", "phpstan.neon"))
		}
		if logging.IsVerbose(ctx) {
			args = append(args, "-v")
		}
		cmd := CommandContext(ctx, "php", args...)
		cmd.Env = append(os.Environ(), "PHP_DIR="+path.Join(config.ToolDirectory, "php"))
		cmd.Dir = config.RootDir
		subprocess := execution.Run(OperationAnalyse, cmd)
		stdout, stderr, runErr := []byte(subprocess.Stdout), []byte(subprocess.Stderr), subprocess.Err
		if !expectedExit(runErr, 1) {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run phpstan in %s: %w", directory, runErr)))
			continue
		}
		if isPhpStanNoFilesOutput(string(stdout)) || isPhpStanNoFilesOutput(string(stderr)) {
			continue
		}
		analysed = true
		output := strings.ReplaceAll(string(stdout), `"files":[]`, `"files":{}`)
		var parsed PhpStanOutput
		if err := json.Unmarshal([]byte(output), &parsed); err != nil {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse phpstan output: %w", err)))
			continue
		}
		if parsed.Files == nil {
			execution.Fail(subprocess.ToolError(errors.New("cannot parse phpstan output: missing file results")))
			continue
		}
		for _, message := range parsed.Errors {
			execution.Fail(subprocess.ToolError(fmt.Errorf("cannot complete phpstan analysis: %s", message)))
		}
		for fileName, file := range parsed.Files {
			for _, message := range file.Messages {
				if strings.HasSuffix(message.Identifier, "deprecated") && p.isUselessDeprecation(message.Message) {
					continue
				}
				execution.AddFindings(validation.CheckResult{
					Path: validation.NormalizeSourcePath(fileName, config.RootDir), Line: message.Line,
					Message: message.Message, Severity: validation.SeverityError,
					Identifier: "phpstan/" + message.Identifier, Tip: message.Tip,
				})
			}
		}
	}
	result := ToolSummary{Note: note}
	if !analysed {
		result.SkipReason = "no files to analyse"
	}
	return result
}

func isPhpStanNoFilesOutput(output string) bool {
	return strings.Contains(output, "No files found to analyse")
}

var tagPartRegex = regexp.MustCompile(`tag:v[0-9]+.[0-9]+.[0-9]+`)
var parameterRemovedRegex = regexp.MustCompile("Parameter.*will be removed")

func (p PhpStan) isUselessDeprecation(message string) bool {
	if !tagPartRegex.MatchString(message) {
		return true
	}

	if parameterRemovedRegex.MatchString(message) {
		return true
	}

	if strings.Contains(message, "reason:return-type-change") ||
		strings.Contains(message, "reason:new-optional-parameter") ||
		strings.Contains(message, "reason:exception-change") {
		return true
	}

	return false
}

func init() {
	AddTool(PhpStan{})
}
