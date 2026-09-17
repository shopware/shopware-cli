package projectbuild

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/shyim/go-composer"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

type ContainerOptions struct {
	PHPVersion          string
	WithDevDependencies bool
	// These options only affect the command printed for the user.
	Tags     []string
	Platform string
	Load     bool
	Push     bool
}

// GenerateContainerFiles writes standalone, committable build files into root.
// It does not stage the project, resolve credentials, or invoke Docker.
func GenerateContainerFiles(ctx context.Context, root string, cfg *shop.Config, opts ContainerOptions) ([]string, error) {
	for _, tag := range opts.Tags {
		if strings.TrimSpace(tag) == "" {
			return nil, errors.New("image tag must not be empty")
		}
	}
	phpVersion, err := containerPHPVersion(root, cfg, opts)
	if err != nil {
		return nil, err
	}
	dockerfile, err := shop.ProductionDockerfile(shop.ProductionDockerfileOptions{
		PHPVersion:          phpVersion,
		WithDevDependencies: opts.WithDevDependencies,
	})
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	files := []struct {
		path    string
		content []byte
	}{
		{filepath.Join(root, "Dockerfile"), dockerfile},
		{filepath.Join(root, ".dockerignore"), defaultContainerIgnore(cfg)},
	}
	// Check both paths before writing either. Lstat also detects dangling links.
	for _, file := range files {
		if _, err := os.Lstat(file.path); err == nil {
			return nil, fmt.Errorf("refusing to overwrite existing file %q", file.path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	var created []string
	complete := false
	defer func() {
		if !complete {
			for _, path := range created {
				if err := os.Remove(path); err != nil {
					logging.FromContext(ctx).Warnf("Could not remove generated file %s: %v", path, err)
				}
			}
		}
	}()
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// O_EXCL ensures a file created after the preflight check is also safe.
		output, err := os.OpenFile(file.path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return nil, fmt.Errorf("create %s: %w", file.path, err)
		}
		created = append(created, file.path)
		_, writeErr := output.Write(file.content)
		if err := errors.Join(writeErr, output.Close()); err != nil {
			return nil, fmt.Errorf("write %s: %w", file.path, err)
		}
	}
	complete = true
	return created, nil
}

// ContainerBuildCommand formats a POSIX-shell command; it never executes it.
// Quote all user-supplied arguments so paths and tags cannot become shell syntax.
func ContainerBuildCommand(root string, opts ContainerOptions) string {
	quote := func(value string) string {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	args := []string{"docker", "buildx", "build"}
	for _, tag := range opts.Tags {
		args = append(args, "--tag", quote(tag))
	}
	if opts.Platform != "" {
		args = append(args, "--platform", quote(opts.Platform))
	}
	if opts.Load {
		args = append(args, "--load")
	}
	if opts.Push {
		args = append(args, "--push")
	}
	args = append(args, quote(root))
	return strings.Join(args, " ")
}

func containerPHPVersion(root string, cfg *shop.Config, opts ContainerOptions) (string, error) {
	if opts.PHPVersion != "" {
		return opts.PHPVersion, nil
	}
	if cfg.PHPVersion != "" {
		return cfg.PHPVersion, nil
	}
	if cfg.Docker != nil && cfg.Docker.PHP != nil && cfg.Docker.PHP.Version != "" {
		return cfg.Docker.PHP.Version, nil
	}

	// Read from the selected project, not the caller's working directory.
	lock, err := composer.ReadLock(filepath.Join(root, "composer.lock"))
	if errors.Is(err, os.ErrNotExist) {
		return "8.3", nil
	}
	if err != nil {
		return "", fmt.Errorf("detect container PHP version from composer.lock: %w", err)
	}
	constraint := shop.ShopwarePHPConstraint(lock)
	if constraint == nil {
		return "8.3", nil
	}
	phpVersion := constraint.HighestSupported()
	// The shared selector has a best-effort fallback for interactive setup.
	// Do not silently generate an incompatible image when no series matches.
	if !constraint.Check(phpVersion + ".0") {
		return "", fmt.Errorf("no supported PHP image version satisfies the locked Shopware requirement %q; specify --php-version explicitly", constraint.String())
	}
	return phpVersion, nil
}

func defaultContainerIgnore(cfg *shop.Config) []byte {
	content := []byte(shop.ProductionDockerignore())
	if cfg.DisableComposerInstall {
		// A prebuilt vendor tree is needed when Composer installation is off.
		content = append(content, []byte("\n!vendor\n!vendor/**\n")...)
	}
	if cfg.Build != nil && cfg.Build.DisableAssetCopy {
		content = append(content, []byte("\n!public/bundles\n!public/bundles/**\n")...)
	}
	return content
}
