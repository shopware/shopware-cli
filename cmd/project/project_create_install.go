package project

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/spf13/cobra"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

func installAndFinalize(cmd *cobra.Command, opts *createOptions, phpConstraint *shop.PHPConstraint, chosenVersion string) error {
	ctx := cmd.Context()

	logging.FromContext(ctx).Infof("Installing dependencies")

	showSpinner := system.IsInteractionEnabled(ctx) && !opts.isVerbose

	composerInstallPHP := ""
	if opts.useDocker {
		composerInstallPHP = phpConstraint.HighestSupported()
		logging.FromContext(ctx).Infof("Using PHP %s for composer install", composerInstallPHP)
	}

	if output, err := runComposerInstall(ctx, opts.projectFolder, opts.useDocker, showSpinner, composerInstallPHP); err != nil {
		if !isComposerSecurityBlocked(output) || opts.noAudit {
			return err
		}

		if err := handleSecurityBlockedInstall(ctx, opts, chosenVersion); err != nil {
			return err
		}

		if _, err := runComposerInstall(ctx, opts.projectFolder, opts.useDocker, showSpinner, composerInstallPHP); err != nil {
			return err
		}
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
	projectDisplay := opts.projectFolder
	if projectDisplay == "." {
		if wd, err := os.Getwd(); err == nil {
			projectDisplay = wd
		}
	}

	if !opts.interactive {
		logging.FromContext(ctx).Infof("Project created successfully in %s", projectDisplay)
		return
	}

	fmt.Println()
	fmt.Println(tui.GreenText.Render("✔ Setup complete in " + projectDisplay))

	if opts.useDocker {
		fmt.Println()
		fmt.Println(tui.SectionHeadingStyle.Render("Next steps"))
		fmt.Println()
		if opts.projectFolder == "." {
			fmt.Printf("  %s  %s\n", tui.GreenText.Render("Start developing:"), tui.BoldText.Render("shopware-cli project dev"))
		} else {
			fmt.Printf("  %s  %s\n", tui.GreenText.Render("Start developing:"), tui.BoldText.Render(fmt.Sprintf("cd %s && shopware-cli project dev", opts.projectFolder)))
		}
		fmt.Println()
		fmt.Println(tui.SectionHeadingStyle.Render("Access your shop (after make setup)"))
		fmt.Println()
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Storefront:"), tui.BoldText.Render("http://127.0.0.1:8000"))
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Admin:"), tui.BoldText.Render("http://127.0.0.1:8000/admin"))
		fmt.Printf("  %s  %s\n", tui.GreenText.Render("Credentials:"), tui.BoldText.Render("admin")+" / "+tui.BoldText.Render("shopware"))
	}

	fmt.Println()
}

// isComposerSecurityBlocked reports whether composer output indicates that
// dependency resolution failed because packages affected by security
// advisories were blocked (composer >= 2.9 with audit blocking enabled).
func isComposerSecurityBlocked(output string) bool {
	return strings.Contains(output, "affected by security advisories")
}

// handleSecurityBlockedInstall is called when composer refused to install
// dependencies affected by security advisories. Interactively it asks the user
// whether to continue without audit blocking and rewrites composer.json with
// audit blocking disabled; non-interactively it fails with a --no-audit hint.
func handleSecurityBlockedInstall(ctx context.Context, opts *createOptions, chosenVersion string) error {
	if !opts.interactive {
		return fmt.Errorf("dependencies of Shopware %s are affected by known security advisories; re-run with --no-audit to proceed. We strongly recommend installing the Shopware Security plugin (https://store.shopware.com/en/swag136939272659f/shopware-6-security-plugin.html) which backports security fixes to older versions", chosenVersion)
	}

	var continueAnyway string
	if err := huh.NewForm(huh.NewGroup(
		tui.NewYesNo().
			Title(fmt.Sprintf("Dependencies of Shopware %s are affected by known security advisories", chosenVersion)).
			Description("Composer refused to install packages affected by security advisories. Continuing will disable composer's audit blocking (--no-audit) so installation can proceed. If you continue, we strongly recommend installing the Shopware Security plugin (https://store.shopware.com/en/swag136939272659f/shopware-6-security-plugin.html) which backports security fixes to older versions. Do you want to continue anyway?").
			Value(&continueAnyway),
	)).Run(); err != nil {
		return err
	}

	if continueAnyway == tui.No {
		return fmt.Errorf("project creation cancelled")
	}

	opts.noAudit = true
	scaffold := newShopwareProjectScaffold(opts, chosenVersion)
	if err := scaffold.WriteComposerJson(ctx); err != nil {
		return err
	}

	logging.FromContext(ctx).Infof("Retrying installation with audit blocking disabled")
	return nil
}

func runComposerInstall(ctx context.Context, projectFolder string, useDocker bool, showSpinner bool, phpVersion string) (string, error) {
	var cmdInstall *exec.Cmd

	if useDocker && !system.IsInsideContainer() {
		absProjectFolder, err := filepath.Abs(projectFolder)
		if err != nil {
			return "", err
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
			phpVersion = shop.SupportedPHPVersions[len(shop.SupportedPHPVersions)-1]
		}
		dockerArgs = append(dockerArgs,
			fmt.Sprintf("ghcr.io/shopware/docker-dev:php%s-node24-caddy", phpVersion),
			"composer", "install", "--no-interaction")

		cmdInstall = exec.CommandContext(ctx, "docker", dockerArgs...)
	} else {
		composerBinary, err := exec.LookPath("composer")
		if err != nil {
			return "", err
		}

		phpBinary := os.Getenv("PHP_BINARY")

		if phpBinary != "" {
			cmdInstall = exec.CommandContext(ctx, phpBinary, composerBinary, "install", "--no-interaction")
		} else {
			cmdInstall = exec.CommandContext(ctx, "composer", "install", "--no-interaction")
		}

		cmdInstall.Dir = projectFolder
	}

	var output bytes.Buffer

	if !showSpinner {
		cmdInstall.Stdin = os.Stdin
		cmdInstall.Stdout = io.MultiWriter(os.Stdout, &output)
		cmdInstall.Stderr = io.MultiWriter(os.Stderr, &output)

		err := cmdInstall.Run()
		return output.String(), err
	}

	cmdInstall.Stdout = &output
	cmdInstall.Stderr = &output

	err := tui.RunSpinnerWithLogs(ctx, "Installing dependencies", cmdInstall)
	return output.String(), err
}
