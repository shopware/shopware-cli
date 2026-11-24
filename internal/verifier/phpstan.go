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
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

var possiblePHPStanConfigs = []string{
	"phpstan.neon",
	"phpstan.neon.dist",
	"phpstan.dist.neon",
}

const phpstanFutureRuleset = "vendor/shopwarelabs/phpstan-shopware/future-compatibility.neon"

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

func (p PhpStan) configFile(pluginPath string) string {
	for _, config := range possiblePHPStanConfigs {
		if _, err := os.Stat(path.Join(pluginPath, config)); err == nil {
			return path.Join(pluginPath, config)
		}
	}

	return ""
}

func (p PhpStan) Check(ctx context.Context, config ToolConfig, execution *ToolExecution) ToolSummary {
	if result, applicable := preparePHPTool(config, execution); !applicable {
		return result
	}
	phpDir := path.Join(config.ToolDirectory, "php")
	nextMajor := config.CheckAgainst == CheckAgainstNextMajor
	if nextMajor {
		if _, err := os.Stat(path.Join(phpDir, phpstanFutureRuleset)); err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot load PHPStan's next-major ruleset, refresh the verifier tools: %w", err)})
			return ToolSummary{}
		}
	}
	autoloadFile, toolErr := prepareComposerAutoload(ctx, config, execution)
	if toolErr != nil {
		execution.Fail(toolErr)
		return ToolSummary{}
	}
	result := ToolSummary{}
	if installed := shop.DetectShopwareVersion(path.Dir(path.Dir(autoloadFile))); installed != "" {
		result.Note = "Shopware " + installed
		if version := tracking.ReportableShopwareVersion(installed); version != "" {
			tracking.SetCommandTag(tracking.TagCheckedShopwareVersion, version)
		}
		if !config.IsProject || nextMajor {
			mode := config.CheckAgainst
			if mode == "" {
				mode = CheckAgainstHighest
			}
			result.Note += " (" + mode + ")"
			tracking.SetCommandTag(tracking.TagCheckAgainst, mode)
		}
	}
	// All source directories go into one run, so PHPStan knows the classes of
	// each of them while analysing the others.
	args := []string{"-dmemory_limit=2G", path.Join(phpDir, "vendor", "bin", "phpstan"), "analyse", "--no-progress", "--no-interaction", "--error-format=json", "--autoload-file", autoloadFile}
	args = append(args, config.SourceDirectories...)
	env := append(os.Environ(), "PHP_DIR="+phpDir)
	// Select the source configuration explicitly because PHPStan runs from
	// the dependency directory so it discovers the matching Composer metadata.
	configFile := p.configFile(config.RootDir)
	// A local phpstan.neon brings its own tmpDir.
	if configFile == "" {
		cacheDir, err := ensureValidationCacheDir(config.RootDir)
		if err != nil {
			execution.Fail(&ToolError{Err: err})
			return result
		}
		configFile = path.Join(phpDir, "configs", "phpstan.neon")
		env = append(env, "SHOPWARE_CLI_PHPSTAN_TMP_DIR="+path.Join(cacheDir, "phpstan"))
	}
	if nextMajor {
		wrapper, err := phpstanNextMajorConfig()
		if err != nil {
			execution.Fail(&ToolError{Err: fmt.Errorf("cannot create PHPStan's next-major configuration: %w", err)})
			return result
		}
		defer func() { _ = os.Remove(wrapper) }()
		env = append(env, "SHOPWARE_CLI_PHPSTAN_BASE_CONFIG="+configFile)
		configFile = wrapper
	}
	args = append(args, "--configuration", configFile)
	if logging.IsVerbose(ctx) {
		args = append(args, "-v")
	}
	cmd := CommandContext(ctx, "php", args...)
	cmd.Env = env
	cmd.Dir = path.Dir(path.Dir(autoloadFile))
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

func phpstanNextMajorConfig() (string, error) {
	file, err := os.CreateTemp("", "shopware-cli-phpstan-*.neon")
	if err != nil {
		return "", err
	}
	// Let PHPStan expand the paths, so file names do not need NEON escaping.
	_, writeErr := file.WriteString(`includes:
    - %env.SHOPWARE_CLI_PHPSTAN_BASE_CONFIG%
    - %env.PHP_DIR%/` + phpstanFutureRuleset + "\n")
	if err := errors.Join(writeErr, file.Close()); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
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
