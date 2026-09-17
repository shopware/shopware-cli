package projectbuild

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	cp "github.com/otiai10/copy"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

type ArchiveOptions struct {
	// OutputPath is relative to the caller's working directory. An empty path
	// creates a uniquely named artifact in root/.shopware-cli/deployments.
	OutputPath string
	// ConfigPath identifies a custom project config to omit from the archive.
	// The resolved deployment configuration is written separately.
	ConfigPath string
	Build      Options
}

// PackageArchive builds a private, temporary copy and publishes a tar.gz.
// Existing output files are never overwritten. This does not deploy anything.
// Hooks are trusted project code, not sandboxed processes.
func PackageArchive(ctx context.Context, root string, cfg *shop.Config, env *shop.EnvironmentConfig, opts ArchiveOptions) (string, error) {
	opts.Build.MirrorPathRepositories = true
	return packageArchive(ctx, root, cfg, opts, func(ctx context.Context, stage string) error {
		return Build(ctx, stage, cfg, env, opts.Build)
	})
}

func packageArchive(ctx context.Context, root string, cfg *shop.Config, opts ArchiveOptions, build func(context.Context, string) error) (string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path %q is not a directory", root)
	}
	output := opts.OutputPath
	if output == "" {
		output = filepath.Join(root, ".shopware-cli", "deployments", "shopware-"+rand.Text()+".tar.gz")
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(output); err == nil {
		return "", fmt.Errorf("archive output %q already exists", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	temp, err := os.MkdirTemp("", "shopware-deployment-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := os.RemoveAll(temp); err != nil {
			logging.FromContext(ctx).Warnf("Could not remove temporary build directory %s: %v", temp, err)
		}
	}()
	// TMPDIR may itself be inside the project; never copy the staging tree.
	temp, err = filepath.EvalSymlinks(temp)
	if err != nil {
		return "", err
	}
	stage := filepath.Join(temp, "project")
	if err := copyPackageSource(ctx, root, stage, temp, func(name string) bool { return archiveExcluded(name, false) }); err != nil {
		return "", fmt.Errorf("copy project: %w", err)
	}
	if err := validateBuildSymlinks(ctx, stage); err != nil {
		return "", fmt.Errorf("validate archive source: %w", err)
	}
	if err := build(ctx, stage); err != nil {
		return "", fmt.Errorf("build archive: %w", err)
	}

	if err := writeDeploymentConfig(stage, cfg); err != nil {
		return "", err
	}
	excludedConfigs := packageConfigPaths(root, cfg, opts.ConfigPath)
	exclude := func(name string) bool {
		return archiveExcluded(name, true) || excludedConfigs[name]
	}
	if err := publishArchive(ctx, stage, output, exclude); err != nil {
		return "", err
	}
	return output, nil
}

// Do not distribute CLI credentials or machine-specific environment config.
// Deployment Helper only needs the resolved deployment section.
func writeDeploymentConfig(stage string, cfg *shop.Config) error {
	for _, name := range []string{".shopware-project.yml", ".shopware-project.yaml"} {
		if err := os.Remove(filepath.Join(stage, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if cfg.ConfigDeployment != nil {
		return shop.WriteConfig(&shop.Config{
			CompatibilityDate: cfg.CompatibilityDate,
			ConfigDeployment:  cfg.ConfigDeployment,
		}, stage)
	}
	return nil
}

// copyPackageSource copies links without following them. The caller validates
// the copied links before building in the staging tree.
func copyPackageSource(ctx context.Context, root, stage, temp string, exclude func(string) bool) error {
	return cp.Copy(root, stage, cp.Options{
		PreserveTimes: true,
		OnSymlink:     func(string) cp.SymlinkAction { return cp.Shallow },
		WrapReader: func(r io.Reader) io.Reader {
			return archiver.ContextReader(ctx, r)
		},
		Skip: func(info os.FileInfo, source, _ string) (bool, error) {
			if err := ctx.Err(); err != nil {
				return false, err
			}
			if source == temp {
				return true, nil
			}
			rel, err := filepath.Rel(root, source)
			if err != nil {
				return false, err
			}
			if exclude(filepath.ToSlash(rel)) {
				return true, nil
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return false, nil
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return false, fmt.Errorf("cannot copy special file %q", rel)
			}
			return false, nil
		},
	})
}

func validateBuildSymlinks(ctx context.Context, root string) error {
	return filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			_, err := archiver.ContainedSymlink(root, name)
			return err
		}
		return nil
	})
}

// Apply the same exclusions before and after the build, since hooks may create
// new runtime files. Composer auth is available only in the private build tree.
func archiveExcluded(name string, final bool) bool {
	base := filepath.Base(name)
	if base == ".git" || base == ".shopware-cli" {
		return true
	}
	if (base == ".env" || strings.HasPrefix(base, ".env.")) && base != ".env.dist" && base != ".env.example" {
		return true
	}
	if strings.HasPrefix(base, ".shopware-project.local.") {
		return true
	}
	if final && base == "auth.json" {
		return true
	}
	switch name {
	case "var", "public/media", "public/thumbnail", "public/sitemap":
		return true
	}
	return false
}

func packageConfigPaths(root string, cfg *shop.Config, configPath string) map[string]bool {
	result := make(map[string]bool)
	configs := []string{configPath, filepath.Join(root, ".shopware-project.yml"), filepath.Join(root, ".shopware-project.yaml")}
	for _, name := range append(configs, cfg.AdditionalConfigs...) {
		if name == "" {
			continue
		}
		for _, configFile := range []string{name, shop.LocalConfigFileName(name)} {
			absolute, err := filepath.Abs(configFile)
			if err != nil {
				continue
			}
			// Resolve the parent to handle aliases such as /tmp on macOS,
			// while still excluding a config file which is itself a symlink.
			parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
			if err != nil {
				continue
			}
			absolute = filepath.Join(parent, filepath.Base(absolute))
			target, _ := filepath.EvalSymlinks(absolute)
			for _, candidate := range []string{absolute, target} {
				rel, err := filepath.Rel(root, candidate)
				if err != nil || !filepath.IsLocal(rel) {
					continue
				}
				// These have already been replaced with deployment-only config.
				if rel != ".shopware-project.yml" && rel != ".shopware-project.yaml" {
					result[filepath.ToSlash(rel)] = true
				}
			}
		}
	}
	return result
}

func publishArchive(ctx context.Context, stage, output string, exclude func(string) bool) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(output), ".shopware-archive-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file.Name()) }()
	writeErr := archiver.WriteTarGz(ctx, file, stage, exclude)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write archive: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Publish only a complete artifact, atomically and without replacing an
	// existing reference (including one created while the build was running).
	if err := os.Link(file.Name(), output); err != nil {
		return fmt.Errorf("publish archive %q: %w", output, err)
	}
	return nil
}
