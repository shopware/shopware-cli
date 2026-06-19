package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

func installAndFinalize(cmd *cobra.Command, opts *createOptions, phpConstraint *packagist.PHPConstraint) error {
	ctx := cmd.Context()

	logging.FromContext(ctx).Infof("Installing dependencies")

	showSpinner := system.IsInteractionEnabled(ctx) && !opts.isVerbose

	composerInstallPHP := ""
	if opts.useDocker {
		composerInstallPHP = phpConstraint.HighestSupported()
		logging.FromContext(ctx).Infof("Using PHP %s for composer install", composerInstallPHP)
	}

	if err := runComposerInstall(ctx, opts.projectFolder, opts.useDocker, showSpinner, composerInstallPHP); err != nil {
		return err
	}

	if opts.useDocker {
		if err := dockerpkg.WriteComposeFile(opts.projectFolder, &dockerpkg.ComposeOptions{PHPVersion: composerInstallPHP}); err != nil {
			return err
		}
	}

	if opts.initGit {
		logging.FromContext(ctx).Infof("Initializing Git repository")
		if err := git.Init(ctx, opts.projectFolder); err != nil {
			return fmt.Errorf("failed to initialize git repository: %w", err)
		}
	}

	shopCfg := shop.NewConfig()
	if opts.useDocker {
		shopCfg.Environments["local"].Type = "docker"
		shopCfg.Docker = &shop.ConfigDocker{
			PHP: &shop.ConfigDockerPHP{Version: composerInstallPHP},
		}
	}

	if err := shop.WriteConfig(shopCfg, opts.projectFolder); err != nil {
		return err
	}

	printCreateSummary(ctx, opts)
	return nil
}

func printCreateSummary(ctx context.Context, opts *createOptions) {
	if !opts.interactive {
		logging.FromContext(ctx).Infof("Project created successfully in %s", opts.projectFolder)
		return
	}

	cmdStyle := lipgloss.NewStyle().Bold(true)
	sectionStyle := lipgloss.NewStyle().Bold(true).Underline(true)

	fmt.Println()
	fmt.Println(tui.GreenText.Render("✔ Setup complete in " + opts.projectFolder))

	if opts.useDocker {
		fmt.Println()
		fmt.Println(sectionStyle.Render("Next steps"))
		fmt.Println()
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Start developing:"), cmdStyle.Render(fmt.Sprintf("cd %s && shopware-cli project dev", opts.projectFolder)))
		fmt.Println()
		fmt.Println(sectionStyle.Render("Access your shop (after make setup)"))
		fmt.Println()
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Storefront:"), cmdStyle.Render("http://127.0.0.1:8000"))
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Admin:"), cmdStyle.Render("http://127.0.0.1:8000/admin"))
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Credentials:"), cmdStyle.Render("admin")+" / "+cmdStyle.Render("shopware"))
	}

	fmt.Println()
}

func runComposerInstall(ctx context.Context, projectFolder string, useDocker bool, showSpinner bool, phpVersion string) error {
	var cmdInstall *exec.Cmd

	if useDocker && !system.IsInsideContainer() {
		absProjectFolder, err := filepath.Abs(projectFolder)
		if err != nil {
			return err
		}

		dockerArgs := []string{"run",
			"--rm",
			"--pull=always",
			"-v", fmt.Sprintf("%s:/app", absProjectFolder),
			"-w", "/app"}

		dockerArgs = append(dockerArgs, system.DockerRunUserArgs(absProjectFolder)...)

		if system.IsDockerMountable() {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				composerDir := filepath.Join(homeDir, ".composer")
				_ = os.MkdirAll(composerDir, 0o755)
				dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/tmp/composer/", composerDir))
			}
		}

		if phpVersion == "" {
			phpVersion = packagist.SupportedPHPVersions[len(packagist.SupportedPHPVersions)-1]
		}
		dockerArgs = append(dockerArgs,
			fmt.Sprintf("ghcr.io/shopware/docker-dev:php%s-node24-caddy", phpVersion),
			"composer", "install", "--no-interaction")

		cmdInstall = exec.CommandContext(ctx, "docker", dockerArgs...)
	} else {
		composerBinary, err := exec.LookPath("composer")
		if err != nil {
			return err
		}

		phpBinary := os.Getenv("PHP_BINARY")

		if phpBinary != "" {
			cmdInstall = exec.CommandContext(ctx, phpBinary, composerBinary, "install", "--no-interaction")
		} else {
			cmdInstall = exec.CommandContext(ctx, "composer", "install", "--no-interaction")
		}

		cmdInstall.Dir = projectFolder
	}

	if !showSpinner {
		cmdInstall.Stdin = os.Stdin
		cmdInstall.Stdout = os.Stdout
		cmdInstall.Stderr = os.Stderr

		return cmdInstall.Run()
	}

	// Interactive, non-verbose mode: show a spinner with a live log tail the
	// user can toggle to follow the (potentially long-running) install.
	return runInstallWithLiveLog(ctx, "Installing dependencies", cmdInstall)
}
