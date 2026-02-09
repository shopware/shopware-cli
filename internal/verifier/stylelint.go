package verifier

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"

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
	ParseErrors           []any          `json:"parseErrors"`
	Errored               bool           `json:"errored"`
	Warnings              []StylintError `json:"warnings"`
}

type StyleLint struct{}

func (s StyleLint) Name() string {
	return "stylelint"
}

func (s StyleLint) Check(ctx context.Context, check *Check, config ToolConfig) error {
	paths := append([]string{}, config.StorefrontDirectories...)
	paths = append(paths, config.AdminDirectories...)

	var gr errgroup.Group

	for _, p := range paths {
		p := p

		hasSCSS, err := hasSCSSFiles(p)
		if err != nil {
			return err
		}

		if !hasSCSS {
			continue
		}

		gr.Go(func() error {
			stylelint := exec.CommandContext(ctx, "node", path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "stylelint"),
				"--formatter=json",
				"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("stylelint.config.%s.mjs", path.Base(p))),
				"--ignore-pattern", "dist/**",
				"--ignore-pattern", ".tmp/**",
				"--ignore-pattern", "vendor/**",
				fmt.Sprintf("%s/**/*.scss", p),
			)
			stylelint.Dir = p

			log, _ := stylelint.CombinedOutput()

			var stylelintOutput StylelintOutput

			if err := json.Unmarshal(log, &stylelintOutput); err != nil {
				return fmt.Errorf("failed to unmarshal stylelint output: %w, %s", err, string(log))
			}

			for _, diagnostic := range stylelintOutput {
				fixedPath := strings.TrimPrefix(strings.TrimPrefix(diagnostic.Source, "/private"), config.RootDir+"/")

				for _, msg := range diagnostic.Warnings {
					check.AddResult(validation.CheckResult{
						Path:       fixedPath,
						Line:       msg.Line,
						Message:    msg.Text,
						Severity:   msg.Severity,
						Identifier: fmt.Sprintf("stylelint/%s", msg.Rule),
					})
				}

				for _, msg := range diagnostic.Deprecations {
					check.AddResult(validation.CheckResult{
						Path:       fixedPath,
						Line:       msg.Line,
						Message:    msg.Text,
						Severity:   msg.Severity,
						Identifier: fmt.Sprintf("stylelint/%s", msg.Rule),
					})
				}
			}

			return nil
		})
	}

	return gr.Wait()
}

func (s StyleLint) Fix(ctx context.Context, config ToolConfig) error {
	paths := append([]string{}, config.StorefrontDirectories...)
	paths = append(paths, config.AdminDirectories...)

	var gr errgroup.Group

	for _, p := range paths {
		p := p

		hasSCSS, err := hasSCSSFiles(p)
		if err != nil {
			return err
		}

		if !hasSCSS {
			continue
		}

		gr.Go(func() error {
			stylelint := exec.CommandContext(ctx, "node", path.Join(config.ToolDirectory, "js", "node_modules", ".bin", "stylelint"),
				"--config", path.Join(config.ToolDirectory, "js", "configs", fmt.Sprintf("stylelint.config.%s.mjs", path.Base(p))),
				"--ignore-pattern", "dist/**",
				"--ignore-pattern", ".tmp/**",
				"--ignore-pattern", "vendor/**",
				"**/*.scss",
				"--fix",
			)
			stylelint.Dir = p
			stylelint.Stdout = os.Stdout
			stylelint.Stderr = os.Stderr

			return stylelint.Run()
		})
	}

	return gr.Wait()
}

func (s StyleLint) Format(ctx context.Context, config ToolConfig, dryRun bool) error {
	return nil
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
