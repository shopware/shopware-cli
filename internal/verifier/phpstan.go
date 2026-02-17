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
		//nolint: nilerr
		return nil
	}

	if err := installComposerDeps(ctx, config.RootDir, config.CheckAgainst); err != nil {
		return err
	}

	for _, sourceDirectory := range config.SourceDirectories {
		phpstanArguments := []string{"-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "phpstan"), "analyse", "--no-progress", "--no-interaction", "--error-format=json", sourceDirectory}

		if !p.configExists(config.RootDir) {
			phpstanArguments = append(phpstanArguments, "--configuration", path.Join(config.ToolDirectory, "php", "configs", "phpstan.neon"))
		}

		phpstan := exec.CommandContext(ctx, "php", phpstanArguments...)
		phpstan.Env = append(os.Environ(), fmt.Sprintf("PHP_DIR=%s", path.Join(config.ToolDirectory, "php")))
		phpstan.Dir = config.RootDir

		var stderr bytes.Buffer
		phpstan.Stderr = &stderr

		log, _ := phpstan.Output()

		log = []byte(strings.ReplaceAll(string(log), "\"files\":[]", "\"files\":{}"))

		var phpstanResult PhpStanOutput

		if err := json.Unmarshal(log, &phpstanResult); err != nil {
			check.AddResult(validation.CheckResult{
				Path:       "phpstan.neon",
				Message:    "failed to unmarshal phpstan output: " + stderr.String(),
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
					Path:       strings.TrimPrefix(strings.TrimPrefix(fileName, "/private"), config.RootDir+"/"),
					Line:       message.Line,
					Message:    message.Message,
					Severity:   validation.SeverityError,
					Identifier: fmt.Sprintf("phpstan/%s", message.Identifier),
					Tip:        message.Tip,
				})
			}
		}
	}

	return nil
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
