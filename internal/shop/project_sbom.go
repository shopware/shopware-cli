package shop

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/sbom"

	"github.com/shopware/shopware-cli/logging"
)

const (
	// DefaultProjectSBOMOutput is the filename written by both `project ci` and
	// `project sbom` when --output is omitted. Keep them identical so tooling
	// that already consumes the CI artifact keeps working.
	DefaultProjectSBOMOutput = "sbom.cdx.json"

	// ProjectSBOMFormatCycloneDXJSON is the only format currently supported.
	// The flag exists so additional formats can be added without breaking the CLI.
	ProjectSBOMFormatCycloneDXJSON = "cyclonedx-json"
)

// ProjectSBOMOptions configures WriteProjectSBOM.
type ProjectSBOMOptions struct {
	// OutputPath is the destination file. Empty means
	// filepath.Join(root, DefaultProjectSBOMOutput). Relative paths are resolved
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

	// ToolVersion is reported in the SBOM metadata tools entry (shopware-cli version).
	ToolVersion string
}

// WriteProjectSBOM reads composer.lock from root and writes a CycloneDX SBOM.
// Shared by `project ci` and `project sbom` so both emit the same document.
func WriteProjectSBOM(ctx context.Context, root string, opts ProjectSBOMOptions) error {
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

	toolVersion := opts.ToolVersion
	if toolVersion == "" {
		toolVersion = "dev"
	}

	bom, err := sbom.Generate(lock, sbom.Options{
		ApplicationName:        appName,
		ApplicationVersion:     appVersion,
		ToolGroup:              "shopware",
		ToolName:               "shopware-cli",
		ToolVersion:            toolVersion,
		IncludeDevDependencies: opts.IncludeDevDependencies,
	})
	if err != nil {
		return err
	}

	data, err := sbom.Marshal(bom)
	if err != nil {
		return fmt.Errorf("marshal SBOM: %w", err)
	}

	outputPath, err := ResolveProjectSBOMOutputPath(root, opts.OutputPath)
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

// ResolveProjectSBOMOutputPath returns the absolute path to write the SBOM to.
// Empty output uses DefaultProjectSBOMOutput under root; absolute paths are kept;
// relative paths resolve against the process working directory.
func ResolveProjectSBOMOutputPath(root, output string) (string, error) {
	if strings.TrimSpace(output) == "" {
		return filepath.Join(root, DefaultProjectSBOMOutput), nil
	}
	if filepath.IsAbs(output) {
		return output, nil
	}
	return filepath.Abs(output)
}

// ValidateProjectSBOMFormat returns an error when format is not supported.
func ValidateProjectSBOMFormat(format string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format != ProjectSBOMFormatCycloneDXJSON {
		return fmt.Errorf("unsupported SBOM format %q (supported: %s)", format, ProjectSBOMFormatCycloneDXJSON)
	}
	return nil
}
