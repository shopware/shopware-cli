package verifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/validation"
)

// rectorOutput supports Rector 2 reports. Empty result lists are omitted by
// Rector, while totals are always present, including when nothing changed.
type rectorOutput struct {
	Totals *struct {
		ChangedFiles *int `json:"changed_files"`
		Errors       *int `json:"errors"`
	} `json:"totals"`
	ChangedFiles []string         `json:"changed_files"`
	FileDiffs    []rectorFileDiff `json:"file_diffs"`
	Errors       []rectorError    `json:"errors"`
}

// rectorFileDiff also represents path-only changes when Rector suppresses diffs.
type rectorFileDiff struct {
	File           string   `json:"file"`
	Diff           string   `json:"diff"`
	AppliedRectors []string `json:"applied_rectors"`
}

type rectorError struct {
	Message  string `json:"message"`
	File     string `json:"file"`
	Line     int    `json:"line"`
	CausedBy string `json:"caused_by"`
}

type Rector struct{}

func (r Rector) Name() string {
	return "rector"
}

func (r Rector) Fix(ctx context.Context, config ToolConfig, execution *ToolExecution, dryRun bool) ToolSummary {
	if result, applicable := preparePHPTool(config, execution); !applicable {
		return result
	}

	// Fix against the minimum compatible dependency set, independently of the
	// extension's locally installed dependencies. Projects keep their vendor/.
	config.CheckAgainst = CheckAgainstLowest
	autoloadFile, toolErr := prepareComposerAutoload(ctx, config, execution)
	if toolErr != nil {
		execution.Fail(toolErr)
		return ToolSummary{}
	}

	configFile, err := r.getConfigPath(config)
	if err != nil {
		execution.Fail(&ToolError{Err: err})
		return ToolSummary{}
	}

	rulesNote := "custom rules from rector.php"
	if configFile != path.Join(config.RootDir, "rector.php") {
		rulesNote = "bundled rules for Shopware " + config.MinShopwareVersion[:3]
	}
	var checkedVersion string
	if installed := shop.DetectShopwareVersion(path.Dir(path.Dir(autoloadFile))); installed != "" {
		note := "Shopware " + installed
		if !config.IsProject {
			note += " (" + CheckAgainstLowest + ")"
			checkedVersion = tracking.ReportableShopwareVersion(installed)
		}
		rulesNote = note + "; " + rulesNote
	}

	result, reportsComplete := r.fixSources(ctx, config, execution, configFile, autoloadFile, dryRun)
	// Report the baseline only once Rector has run against it.
	if reportsComplete && checkedVersion != "" {
		tracking.SetCommandTag(tracking.TagCheckedShopwareVersion, checkedVersion)
		tracking.SetCommandTag(tracking.TagCheckAgainst, CheckAgainstLowest)
	}
	if result.Note != "" {
		result.Note += "; "
	}
	result.Note += rulesNote
	return result
}

func (r Rector) getConfigPath(config ToolConfig) (string, error) {
	localConfig := path.Join(config.RootDir, "rector.php")
	if _, err := os.Stat(localConfig); err == nil {
		return localConfig, nil
	}

	if len(config.MinShopwareVersion) < 3 {
		return "", errors.New("cannot select rector configuration: invalid Shopware version")
	}

	return path.Join(
		config.ToolDirectory, "php", "vendor", "frosh", "shopware-rector", "config",
		fmt.Sprintf("shopware-%s.0.php", config.MinShopwareVersion[:3]),
	), nil
}

// fixSources runs Rector on each source directory. It also reports whether
// every run produced a complete report.
func (r Rector) fixSources(
	ctx context.Context,
	config ToolConfig,
	execution *ToolExecution,
	configFile,
	autoloadFile string,
	dryRun bool,
) (ToolSummary, bool) {
	changed := make(map[string]struct{})
	reportsComplete := true

	for _, sourceDirectory := range config.SourceDirectories {
		report := r.process(ctx, config, execution, configFile, autoloadFile, sourceDirectory, dryRun)
		if report.Totals == nil {
			reportsComplete = false
		}
		if dryRun {
			for _, file := range report.FileDiffs {
				execution.AddFindings(file.pendingFindings(config.RootDir)...)
			}
			continue
		}

		// A write report lists applied edits. Remaining source errors have
		// already been reported by process and are the only normal findings.
		for _, file := range report.FileDiffs {
			changed[filepath.Clean(file.File)] = struct{}{}
		}
	}

	if !dryRun && reportsComplete {
		return ToolSummary{Note: fileChangeNote(len(changed), "fixed")}, true
	}

	return ToolSummary{}, reportsComplete
}

// process interprets one Rector invocation. Syntax errors in analyzed source
// files are findings. Other failures retain their subprocess output because
// Rector also attributes internal rule failures to the input file.
func (r Rector) process(
	ctx context.Context,
	config ToolConfig,
	execution *ToolExecution,
	configFile, autoloadFile, directory string,
	dryRun bool,
) rectorOutput {
	args := []string{
		"-dmemory_limit=2G", path.Join(config.ToolDirectory, "php", "vendor", "bin", "rector"),
		"process", "--config", configFile, "--autoload-file", autoloadFile,
		"--output-format", "json", "--no-ansi", "--no-progress-bar", directory,
	}

	operation := OperationFix
	if dryRun {
		// A preview must analyze the current sources rather than relying on
		// cached results from a previous invocation.
		args = append(args, "--dry-run", "--clear-cache")
		operation = OperationCheck
	}

	cmd := CommandContext(ctx, "php", args...)
	cmd.Dir = path.Dir(path.Dir(autoloadFile))
	subprocess := execution.Run(operation, cmd)

	expectedStatus := expectedExit(subprocess.Err, 1) || (dryRun && expectedExit(subprocess.Err, 2))
	if !expectedStatus {
		execution.Fail(subprocess.ToolError(fmt.Errorf(
			"cannot run rector in %s: %w", directory, subprocess.Err,
		)))
		return rectorOutput{}
	}

	report, err := parseRectorOutput(subprocess.Stdout)
	if err != nil {
		execution.Fail(subprocess.ToolError(fmt.Errorf("cannot parse rector output: %w", err)))
		return rectorOutput{}
	}

	// Rector reports paths relative to its working directory, which is the
	// dependency cache for extensions and the source root for projects.
	for i := range report.FileDiffs {
		if !filepath.IsAbs(report.FileDiffs[i].File) {
			report.FileDiffs[i].File = filepath.Join(cmd.Dir, report.FileDiffs[i].File)
		}
	}
	for i := range report.Errors {
		if report.Errors[i].File != "" && !filepath.IsAbs(report.Errors[i].File) {
			report.Errors[i].File = filepath.Join(cmd.Dir, report.Errors[i].File)
		}
	}

	for _, issue := range report.Errors {
		if finding, ok := issue.sourceFinding(config.RootDir, directory); ok {
			execution.AddFindings(finding)
			continue
		}

		execution.Fail(subprocess.ToolError(issue.processingError(directory)))
	}

	changesPending := dryRun && expectedExit(subprocess.Err, 2) && len(report.FileDiffs) > 0
	if subprocess.Err != nil && len(report.Errors) == 0 && !changesPending {
		execution.Fail(subprocess.ToolError(fmt.Errorf(
			"cannot complete rector analysis: unsuccessful exit without diagnostics: %w", subprocess.Err,
		)))
		return rectorOutput{}
	}

	return report
}

// parseRectorOutput validates totals so incomplete output cannot be mistaken
// for a successful analysis with no pending changes or processing errors.
func parseRectorOutput(output string) (rectorOutput, error) {
	var report rectorOutput
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		return rectorOutput{}, err
	}
	if report.Totals == nil || report.Totals.ChangedFiles == nil || report.Totals.Errors == nil {
		return rectorOutput{}, errors.New("missing result totals")
	}

	for _, file := range report.FileDiffs {
		if strings.TrimSpace(file.File) == "" || strings.TrimSpace(file.Diff) == "" {
			return rectorOutput{}, errors.New("incomplete file diff")
		}
		for _, rule := range file.AppliedRectors {
			if strings.TrimSpace(rule) == "" {
				return rectorOutput{}, errors.New("missing rule in file diff")
			}
		}
	}

	// withNoDiffs() omits file_diffs but still reports every changed path.
	// Normalize those paths for the same summary and verification flow.
	if len(report.FileDiffs) == 0 {
		for _, file := range report.ChangedFiles {
			if strings.TrimSpace(file) == "" {
				return rectorOutput{}, errors.New("missing changed file path")
			}
			report.FileDiffs = append(report.FileDiffs, rectorFileDiff{File: file})
		}
	}

	if *report.Totals.ChangedFiles != len(report.FileDiffs) ||
		*report.Totals.Errors != len(report.Errors) {
		return rectorOutput{}, errors.New("result totals do not match reported changes or errors")
	}

	for _, issue := range report.Errors {
		if strings.TrimSpace(issue.Message) == "" {
			return rectorOutput{}, errors.New("missing processing error message")
		}
	}

	return report, nil
}

// pendingFindings describes changes proposed by a dry run.
func (f rectorFileDiff) pendingFindings(root string) []validation.CheckResult {
	rules := f.AppliedRectors
	if len(rules) == 0 {
		rules = []string{"change"}
	}

	filePath := f.File
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(root, filePath)
	}
	filePath = validation.NormalizeSourcePath(filePath, root)

	findings := make([]validation.CheckResult, 0, len(rules))
	for _, rule := range rules {
		findings = append(findings, validation.CheckResult{
			Path:       filePath,
			Message:    "Rector changes are required. Run fix without dry-run to apply them.",
			Severity:   validation.SeverityError,
			Identifier: "rector/" + rule,
		})
	}

	return findings
}

// sourceFinding recognizes source syntax problems. Rector also assigns source
// paths to internal failures, which must retain their subprocess diagnostics.
func (e rectorError) sourceFinding(root, directory string) (validation.CheckResult, bool) {
	if e.File == "" || e.CausedBy != "" || !strings.HasPrefix(e.Message, "Syntax error") {
		return validation.CheckResult{}, false
	}

	filePath := e.File
	if !filepath.IsAbs(filePath) {
		filePath = filepath.Join(root, filePath)
	}
	if !filepath.IsAbs(directory) {
		directory = filepath.Join(root, directory)
	}
	// Discovery may resolve source directories while the root still uses a
	// symlink, such as macOS /var and /private/var.
	relative, err := filepath.Rel(
		validation.ResolveSourceRoot(directory),
		validation.ResolveSourceRoot(filePath),
	)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return validation.CheckResult{}, false
	}

	return validation.CheckResult{
		Path:       validation.NormalizeSourcePath(filePath, root),
		Line:       e.Line,
		Message:    e.Message,
		Severity:   validation.SeverityError,
		Identifier: "rector/error",
	}, true
}

func (e rectorError) processingError(directory string) error {
	location := e.File
	if location == "" {
		location = directory
	}
	if e.Line > 0 {
		location = fmt.Sprintf("%s:%d", location, e.Line)
	}

	message := e.Message
	if e.CausedBy != "" {
		message = fmt.Sprintf("%s (%s)", message, e.CausedBy)
	}

	return fmt.Errorf("cannot process rector input %s: %s", location, message)
}

func init() {
	AddTool(Rector{})
}
