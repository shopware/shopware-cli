package verifier

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

var possiblePHPStanConfigs = []string{
	"phpstan.neon",
	"phpstan.neon.dist",
	"phpstan.dist.neon",
}

type PhpStanOutput struct {
	Totals struct {
		Errors     int `json:"errors"`
		FileErrors int `json:"file_errors"`
	} `json:"totals"`
	Files map[string]struct {
		Errors   int `json:"errors"`
		Messages []struct {
			Message    string `json:"message"`
			Line       int    `json:"line"`
			Ignorable  bool   `json:"ignorable"`
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

func (p PhpStan) Check(ctx context.Context, check *Check, config ToolConfig) error {
	// Apps don't have an composer.json file, skip them
	if _, err := os.Stat(path.Join(config.RootDir, "composer.json")); err != nil {
		check.RecordToolRun(validation.ToolRun{Name: p.Name(), Status: validation.ToolRunSkipped, Note: "no composer.json"})
		//nolint: nilerr
		return nil
	}

	if err := installComposerDeps(ctx, config.RootDir, config.CheckAgainst, pinnedTarget(config)); err != nil {
		return err
	}

	check.RecordToolRun(phpstanToolRun(config, installedShopwareVersion(config.RootDir)))

	for _, sourceDirectory := range config.SourceDirectories {
		phpstanArguments := []string{"-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "phpstan"), "analyse", "--no-progress", "--no-interaction", "--error-format=json", sourceDirectory}

		if !p.configExists(config.RootDir) {
			phpstanArguments = append(phpstanArguments, "--configuration", path.Join(config.ToolDirectory, "php", "configs", "phpstan.neon"))
		}

		if logging.IsVerbose(ctx) {
			phpstanArguments = append(phpstanArguments, "-v")
		}

		phpstan := exec.CommandContext(ctx, "php", phpstanArguments...)
		phpstan.Env = append(os.Environ(), "PHP_DIR="+path.Join(config.ToolDirectory, "php"))
		phpstan.Dir = config.RootDir

		var stderr bytes.Buffer
		phpstan.Stderr = &stderr

		log, _ := phpstan.Output()

		// When all files of a source directory are excluded by excludePaths in a
		// local phpstan.neon, PHPStan prints a plain text message instead of JSON.
		if isPhpStanNoFilesOutput(string(log)) || isPhpStanNoFilesOutput(stderr.String()) {
			continue
		}

		log = []byte(strings.ReplaceAll(string(log), "\"files\":[]", "\"files\":{}"))

		var phpstanResult PhpStanOutput

		if err := json.Unmarshal(log, &phpstanResult); err != nil {
			errorOutput := stderr.String()
			if strings.TrimSpace(errorOutput) == "" {
				errorOutput = string(log)
			}

			check.AddResult(validation.CheckResult{
				Path:       "phpstan.neon",
				Message:    "failed to unmarshal phpstan output: " + errorOutput,
				Severity:   validation.SeverityError,
				Line:       0,
				Identifier: "phpstan/error",
			})
			//nolint: nilerr
			return nil
		}

		for _, error := range phpstanResult.Errors {
			check.AddResult(validation.CheckResult{
				Path:       "phpstan.neon",
				Message:    error,
				Severity:   validation.SeverityError,
				Line:       0,
				Identifier: "phpstan/error",
			})
		}

		for fileName, file := range phpstanResult.Files {
			for _, message := range file.Messages {
				if strings.HasSuffix(message.Identifier, "deprecated") && p.isUselessDeprecation(message.Message) {
					continue
				}

				check.AddResult(validation.CheckResult{
					Path:       validation.NormalizeSourcePath(fileName, config.RootDir),
					Line:       message.Line,
					Message:    message.Message,
					Severity:   validation.SeverityError,
					Identifier: "phpstan/" + message.Identifier,
					Tip:        message.Tip,
				})
			}
		}
	}

	return nil
}

// pinnedTarget returns the release to install when an explicit target may be installed into a copy.
func pinnedTarget(config ToolConfig) string {
	if config.Target.Source != validation.TargetSourceFlag || !config.RootDirIsCopy || config.Extension == nil {
		return ""
	}

	return config.Target.Version
}

// phpstanToolRun reports the installed shopware/core and how to align it with the baseline.
func phpstanToolRun(config ToolConfig, installed string) validation.ToolRun {
	run := validation.ToolRun{Name: PhpStan{}.Name(), Status: validation.ToolRunRan, Baseline: installed}
	target := config.Target.Version

	if installed == "" || strings.EqualFold(installed, target) {
		return run
	}

	switch {
	case config.Extension == nil:
		// Validation never changes a project's dependencies, so only the project itself can align them
		run.Note = fmt.Sprintf("analysed the installed shopware/core %s; validation does not change project dependencies, so install %s in the project to include PHPStan", installed, target)
	case config.Target.Source == validation.TargetSourceFlag && !config.RootDirIsCopy:
		run.Note = fmt.Sprintf("analysed the installed shopware/core %s; drop --no-copy so %s can be installed in a temporary copy", installed, target)
	case config.Target.Source == validation.TargetSourceFlag:
		run.Note = fmt.Sprintf("analysed the installed shopware/core %s instead of %s, because the extension ships its own vendor directory", installed, target)
	default:
		run.Note = fmt.Sprintf("analysed the installed shopware/core %s; pass --target-version to align all checks", installed)
	}

	return run
}

func isPhpStanNoFilesOutput(output string) bool {
	return strings.Contains(output, "No files found to analyse")
}

func (p PhpStan) Fix(ctx context.Context, config ToolConfig) error {
	return nil
}

func (p PhpStan) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
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
