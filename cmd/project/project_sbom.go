package project

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/sbom"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

const (
	// defaultProjectSBOMOutput is the filename written by both `project ci` and
	// `project sbom` when --output is omitted. Keep them identical so tooling
	// that already consumes the CI artifact keeps working.
	defaultProjectSBOMOutput = "sbom.cdx.json"

	// projectSBOMFormatCycloneDXJSON is the only format currently supported.
	// The flag exists so additional formats can be added without breaking the CLI.
	projectSBOMFormatCycloneDXJSON = "cyclonedx-json"
)

// projectSBOMOptions configures writeProjectSBOM.
type projectSBOMOptions struct {
	// OutputPath is the destination file. Empty means
	// filepath.Join(root, defaultProjectSBOMOutput). Relative paths are resolved
	// against the process working directory (same as other CLI --output flags).
	OutputPath string

	// SkipMissingLock, when true, returns nil without writing a file if
	// composer.lock is absent. Used by `project ci` so a lock-less tree does not
	// fail the build. The standalone command leaves this false so a missing lock
	// is a hard error.
	SkipMissingLock bool

	// IncludeDevDependencies controls whether packages-dev from composer.lock are
	// included. Defaults to false to match `project ci`.
	IncludeDevDependencies bool
}

var projectSbomCmd = &cobra.Command{
	Use:   "sbom [path]",
	Short: "Generate a CycloneDX SBOM from composer.lock",
	Long: `Generate a Software Bill of Materials (SBOM) for a Shopware project.

Reads composer.lock (and optionally composer.json for the root component name
and version) and writes a CycloneDX 1.7 JSON document — the same artifact that
project ci produces, without running the rest of the CI build.

Examples:
  # Write sbom.cdx.json into the current Shopware project
  shopware-cli project sbom

  # Explicit project path and output file
  shopware-cli project sbom ./my-shop \
    --format cyclonedx-json \
    --output sbom.json

The command is non-interactive and exits non-zero when generation fails
(missing or unreadable composer.lock, unsupported format, write errors).`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolveProjectSbomRoot(args)
		if err != nil {
			return err
		}

		format, err := cmd.Flags().GetString("format")
		if err != nil {
			return err
		}
		format = strings.ToLower(strings.TrimSpace(format))
		if format != projectSBOMFormatCycloneDXJSON {
			return fmt.Errorf("unsupported SBOM format %q (supported: %s)", format, projectSBOMFormatCycloneDXJSON)
		}

		output, err := cmd.Flags().GetString("output")
		if err != nil {
			return err
		}

		includeDev, err := cmd.Flags().GetBool("include-dev-dependencies")
		if err != nil {
			return err
		}

		return writeProjectSBOM(cmd.Context(), root, projectSBOMOptions{
			OutputPath:             output,
			SkipMissingLock:        false,
			IncludeDevDependencies: includeDev,
		})
	},
}

func init() {
	projectRootCmd.AddCommand(projectSbomCmd)
	projectSbomCmd.Flags().String("format", projectSBOMFormatCycloneDXJSON, "SBOM format (only cyclonedx-json is supported)")
	projectSbomCmd.Flags().StringP("output", "o", "", fmt.Sprintf("Output file path (default: %s in the project root)", defaultProjectSBOMOutput))
	projectSbomCmd.Flags().Bool("include-dev-dependencies", false, "Include packages-dev from composer.lock (excluded by default, matching project ci)")
}

// resolveProjectSbomRoot picks the project directory: an explicit path argument,
// otherwise the closest Shopware project (composer.json/lock walk), falling back
// to the working directory when no Shopware markers are found further up.
func resolveProjectSbomRoot(args []string) (string, error) {
	if len(args) == 1 {
		return filepath.Abs(args[0])
	}

	root, err := findClosestShopwareProject()
	if err == nil {
		return root, nil
	}

	// findClosestShopwareProject fails when no Shopware markers exist. Still
	// allow generating an SBOM from a plain composer project in cwd so the
	// command is useful outside full Shopware trees.
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		return "", err
	}
	return cwd, nil
}

// writeProjectSBOM reads composer.lock from root and writes a CycloneDX SBOM.
// Shared by `project ci` and `project sbom` so both emit the same document.
func writeProjectSBOM(ctx context.Context, root string, opts projectSBOMOptions) error {
	lockPath := path.Join(root, "composer.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		if opts.SkipMissingLock {
			logging.FromContext(ctx).Infof("Skipping SBOM generation: %s not found", lockPath)
			return nil
		}
		return fmt.Errorf("composer.lock not found at %s; run composer install or pass a project path that contains a lock file", lockPath)
	} else if err != nil {
		return fmt.Errorf("stat composer.lock: %w", err)
	}

	lock, err := composer.ReadLock(lockPath)
	if err != nil {
		return fmt.Errorf("read composer.lock: %w", err)
	}

	projectComposer, err := composer.ReadJson(path.Join(root, "composer.json"))
	appName := "shopware-project"
	appVersion := ""
	if err == nil && projectComposer != nil {
		if projectComposer.Name != "" {
			appName = projectComposer.Name
		}
		if projectComposer.Version != "" {
			appVersion = projectComposer.Version
		}
	}

	bom, err := sbom.Generate(lock, sbom.Options{
		ApplicationName:        appName,
		ApplicationVersion:     appVersion,
		ToolGroup:              "shopware",
		ToolName:               "shopware-cli",
		ToolVersion:            tui.AppVersion,
		IncludeDevDependencies: opts.IncludeDevDependencies,
	})
	if err != nil {
		return err
	}

	data, err := sbom.Marshal(bom)
	if err != nil {
		return fmt.Errorf("marshal SBOM: %w", err)
	}

	outputPath, err := resolveProjectSBOMOutputPath(root, opts.OutputPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create SBOM output directory: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return fmt.Errorf("write SBOM: %w", err)
	}

	logging.FromContext(ctx).Infof("Wrote SBOM with %d components to %s", len(bom.Components), outputPath)
	return nil
}

func resolveProjectSBOMOutputPath(root, output string) (string, error) {
	if strings.TrimSpace(output) == "" {
		return filepath.Join(root, defaultProjectSBOMOutput), nil
	}
	if filepath.IsAbs(output) {
		return output, nil
	}
	// Relative --output is resolved against the process cwd, matching dump and
	// other file-output commands.
	return filepath.Abs(output)
}
