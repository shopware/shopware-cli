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
	_, vendorErr := os.Stat(path.Join(config.RootDir, "vendor"))
	autoloadFile, toolErr := prepareComposerAutoload(ctx, config, execution)
	if toolErr != nil {
		execution.Fail(toolErr)
		return ToolSummary{}
	}
	result := ToolSummary{}
	if installed := shop.DetectShopwareVersion(path.Dir(path.Dir(autoloadFile))); installed != "" {
		result.Note = "Shopware " + installed
		if errors.Is(vendorErr, os.ErrNotExist) && config.CheckAgainst != "" {
			result.Note += " (" + config.CheckAgainst + ")"
		}
	}
	// All source directories go into one run, so PHPStan knows the classes of
	// each of them while analysing the others.
	args := []string{"-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "phpstan"), "analyse", "--no-progress", "--no-interaction", "--error-format=json", "--autoload-file", autoloadFile}
	args = append(args, config.SourceDirectories...)
	env := append(os.Environ(), "PHP_DIR="+path.Join(config.ToolDirectory, "php"))
	// A local phpstan.neon brings its own tmpDir.
	if !p.configExists(config.RootDir) {
		cacheDir, err := ensureValidationCacheDir(config.RootDir)
		if err != nil {
			execution.Fail(&ToolError{Err: err})
			return result
		}
		args = append(args, "--configuration", path.Join(config.ToolDirectory, "php", "configs", "phpstan.neon"))
		env = append(env, "SHOPWARE_CLI_PHPSTAN_TMP_DIR="+path.Join(cacheDir, "phpstan"))
	}
	if logging.IsVerbose(ctx) {
		args = append(args, "-v")
	}
	cmd := CommandContext(ctx, "php", args...)
	cmd.Env = env
	cmd.Dir = config.RootDir
	subprocess := execution.Run(OperationAnalyse, cmd)
	stdout, stderr, runErr := subprocess.Stdout, subprocess.Stderr, subprocess.Err
	if !expectedExit(runErr, 1) {
		execution.Fail(subprocess.ToolError(fmt.Errorf("cannot run phpstan: %w", runErr)))
		return result
	}
	if isPhpStanNoFilesOutput(stdout) || isPhpStanNoFilesOutput(stderr) {
		result.SkipReason = "no files to analyse"
		return result
	}
	output := strings.ReplaceAll(stdout, `"files":[]`, `"files":{}`)
	var parsed PhpStanOutput
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse phpstan output: %w", err)))
		return result
	}
	if parsed.Files == nil {
		execution.Fail(subprocess.ToolError(errors.New("cannot parse phpstan output: missing file results")))
		return result
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
