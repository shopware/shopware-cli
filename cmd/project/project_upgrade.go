package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/flexmigrator"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/projectupgrade"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the Shopware version of this project",
	Long: `Upgrade the Shopware project to a newer version.

In an interactive terminal this starts the upgrade wizard: it runs preflight
safety checks (clean git tree, composer-managed plugins, running environment,
Packagist reachability), lets you pick a target version with release notes,
shows an extension compatibility queue derived from composer's own dry-run
verdict (compatible / will update / deprecated / blocked), and blocks the
upgrade until every blocked extension is either updated or explicitly marked
for removal. A Markdown support report with the findings, composer.json, and
raw composer output can be exported at any time from the review and result
panels.

The upgrade itself resolves incompatible custom plugins, rewrites
composer.json for the new version, runs composer update
--with-all-dependencies, and then invokes vendor/bin/shopware-deployment-helper
to run system:update:prepare, migrations, system:update:finish and the rest of
the deployment lifecycle. shopware/deployment-helper is added to composer.json
automatically when the project doesn't require it yet.

With --to or in non-interactive environments the headless flow runs instead.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		log := logging.FromContext(ctx)

		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		composerJsonPath := path.Join(projectRoot, "composer.json")
		composerLockPath := path.Join(projectRoot, "composer.lock")

		if _, err := os.Stat(composerLockPath); err != nil {
			return fmt.Errorf("composer.lock not found in %s. Run composer install first", projectRoot)
		}

		currentVersion, err := projectupgrade.CurrentShopwareVersion(projectRoot)
		if err != nil {
			return fmt.Errorf("failed to determine current Shopware version: %w", err)
		}

		log.Infof("Current Shopware version: %s", currentVersion.String())

		allowDirty, _ := cmd.Flags().GetBool("allow-dirty")
		allowNonComposer, _ := cmd.Flags().GetBool("allow-non-composer")

		allVersions, err := extension.GetShopwareVersions(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch available Shopware versions: %w", err)
		}

		updateVersions := projectupgrade.FilterUpdateVersions(currentVersion, allVersions)
		if len(updateVersions) == 0 {
			fmt.Println("You are on the latest version of Shopware")
			return nil
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		// Non-interactive: keep the headless flow so CI runs stay unchanged.
		// The safety checks abort hard here; interactively they run as the
		// wizard's preflight checklist instead, with a visible recheck.
		if !system.IsInteractionEnabled(ctx) || cmd.Flag("to").Value.String() != "" {
			if err := ensureCleanGitTree(ctx, projectRoot, allowDirty); err != nil {
				return err
			}
			if err := ensureAllPluginsAreComposerManaged(projectRoot, allowNonComposer); err != nil {
				return err
			}
			return runUpgradeHeadless(cmd, projectRoot, composerJsonPath, currentVersion, updateVersions, cmdExecutor)
		}

		// Interactive: hand off to the devtui-styled wizard.
		result, err := projectupgrade.RunWizard(projectupgrade.WizardOptions{
			ProjectRoot:      projectRoot,
			ComposerJSONPath: composerJsonPath,
			CurrentVersion:   currentVersion,
			UpdateVersions:   updateVersions,
			Executor:         cmdExecutor,
			AllowDirty:       allowDirty,
			AllowNonComposer: allowNonComposer,
		})

		status := "ok"
		switch {
		case errors.Is(err, projectupgrade.ErrCancelled):
			status = "cancelled"
		case err != nil:
			status = "failed"
		case !result.Success:
			status = "failed"
		}

		trackUpgrade(ctx, currentVersion.String(), result.TargetVersion, status)

		// Keep the exported report path in the scrollback; the alt-screen
		// view that showed it is gone once the wizard exits.
		if result.ReportPath != "" {
			fmt.Printf("Upgrade report written to %s\n", result.ReportPath)
		}

		if errors.Is(err, projectupgrade.ErrCancelled) {
			fmt.Println("Upgrade cancelled.")
			return nil
		}

		// The wizard runs in the alt-screen, so its live log is gone once it
		// exits. Replay the full output of the failed task to the terminal so
		// the user keeps the complete error in their scrollback.
		if len(result.FailureLog) > 0 {
			out := cmd.ErrOrStderr()
			_, _ = fmt.Fprintln(out, "\nUpgrade failed. Full output of the failed step:")
			for _, line := range result.FailureLog {
				_, _ = fmt.Fprintln(out, line)
			}
		}

		return err
	},
}

func runUpgradeHeadless(cmd *cobra.Command, projectRoot, composerJsonPath string, currentVersion *version.Version, updateVersions []string, cmdExecutor executor.Executor) error {
	ctx := cmd.Context()
	log := logging.FromContext(ctx)

	targetVersion, err := selectTargetVersion(cmd, updateVersions)
	if err != nil {
		return err
	}

	if err := runCompatibilityCheck(ctx, cmdExecutor, composerJsonPath, targetVersion); err != nil {
		return err
	}

	confirmed := !system.IsInteractionEnabled(ctx)
	if !confirmed {
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Upgrade Shopware from %s to %s?", currentVersion.String(), targetVersion)).
			Description("This will modify composer.json, run composer update --with-all-dependencies, and invoke vendor/bin/shopware-deployment-helper. Commit your changes before running this command.").
			Value(&confirmed).
			Run(); err != nil {
			return err
		}
	}

	if !confirmed {
		return fmt.Errorf("upgrade cancelled")
	}

	log.Infof("Cleaning up stale recipe files")
	if err := flexmigrator.CleanupByHash(projectRoot); err != nil {
		return fmt.Errorf("cleanup stale files: %w", err)
	}

	log.Infof("Resolving plugins with composer for %s", targetVersion)
	result, err := projectupgrade.ApplyRequire(ctx, cmdExecutor, composerJsonPath, targetVersion)
	if err != nil {
		return fmt.Errorf("resolve incompatible plugins: %w", err)
	}

	if result != nil {
		for _, action := range result.Removed() {
			log.Infof("Removed incompatible plugin %s (%s). Re-require it once a compatible version is published.", tui.YellowText.Render(action.Name), action.Reason)
		}
	}

	log.Infof("Updating composer.json to %s", targetVersion)
	if err := projectupgrade.UpdateComposerJson(composerJsonPath, targetVersion); err != nil {
		return fmt.Errorf("update composer.json: %w", err)
	}

	log.Infof("Running composer update")
	composerArgs := []string{
		"update",
		"--no-interaction",
		"--no-scripts",
		"--with-all-dependencies",
		"-v",
	}

	composerCmd := cmdExecutor.ComposerCommand(ctx, composerArgs...)
	composerCmd.Cmd.Stdin = cmd.InOrStdin()
	composerCmd.Cmd.Stdout = cmd.OutOrStdout()
	composerCmd.Cmd.Stderr = cmd.ErrOrStderr()

	if err := composerCmd.Run(); err != nil {
		log.Errorf("composer update failed: %v", err)
		trackUpgrade(ctx, currentVersion.String(), targetVersion, "composer_update_failed")
		return fmt.Errorf("composer update failed: %w", err)
	}

	log.Infof("Running vendor/bin/shopware-deployment-helper run")
	deployCmd := cmdExecutor.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run")
	deployCmd.Cmd.Stdin = cmd.InOrStdin()
	deployCmd.Cmd.Stdout = cmd.OutOrStdout()
	deployCmd.Cmd.Stderr = cmd.ErrOrStderr()

	if err := deployCmd.Run(); err != nil {
		trackUpgrade(ctx, currentVersion.String(), targetVersion, "deployment_helper_failed")
		return fmt.Errorf("shopware-deployment-helper run failed: %w", err)
	}

	trackUpgrade(ctx, currentVersion.String(), targetVersion, "ok")
	fmt.Printf("\n%s\n", tui.GreenText.Render(fmt.Sprintf("Shopware was upgraded from %s to %s", currentVersion.String(), targetVersion)))
	return nil
}

func selectTargetVersion(cmd *cobra.Command, updateVersions []string) (string, error) {
	target, _ := cmd.Flags().GetString("to")
	if target != "" {
		for _, v := range updateVersions {
			if v == target {
				return target, nil
			}
		}
		return "", fmt.Errorf("requested target version %s is not in the list of available upgrade versions", target)
	}

	if !system.IsInteractionEnabled(cmd.Context()) {
		logging.FromContext(cmd.Context()).Infof("Auto selected version %s", updateVersions[0])
		return updateVersions[0], nil
	}

	var selected string
	prompt := huh.NewSelect[string]().
		Height(10).
		Title("Select the Shopware version to upgrade to").
		Options(huh.NewOptions(updateVersions...)...).
		Value(&selected)

	if err := prompt.Run(); err != nil {
		return "", err
	}

	if selected == "" {
		return "", fmt.Errorf("no version selected")
	}

	return selected, nil
}

func runCompatibilityCheck(ctx context.Context, cmdExecutor executor.Executor, composerJsonPath, targetVersion string) error {
	log := logging.FromContext(ctx)

	report, err := projectupgrade.DryRunRequire(ctx, cmdExecutor, composerJsonPath, targetVersion)
	if err != nil {
		log.Warnf("Skipping plugin compatibility check: %v", err)
		return nil
	}

	if report.OK {
		log.Infof("composer can resolve the upgrade to %s", targetVersion)
		return nil
	}

	if len(report.BlockingPlugins) > 0 {
		t := table.New().Border(lipgloss.NormalBorder()).Headers("Plugin", "Status")
		for _, name := range report.BlockingPlugins {
			t.Row(name, "no compatible release")
		}
		fmt.Println(t.Render())
	} else {
		fmt.Println(strings.Join(report.Output, "\n"))
	}

	if system.IsInteractionEnabled(ctx) {
		var proceed bool
		if err := huh.NewConfirm().
			Title("composer could not resolve the upgrade with all plugins").
			Description("Incompatible plugins will be removed from composer.json so the upgrade can proceed. Re-require them once they publish a compatible release. Proceed anyway?").
			Value(&proceed).
			Run(); err != nil {
			return err
		}

		if !proceed {
			return fmt.Errorf("upgrade cancelled due to incompatible plugins")
		}
	}

	return nil
}

func trackUpgrade(ctx context.Context, fromVersion, toVersion, status string) {
	trackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 300*time.Millisecond)
	defer cancel()

	tracking.Track(trackCtx, "project.upgrade", map[string]string{
		"from_version": fromVersion,
		"to_version":   toVersion,
		"status":       status,
		"success":      strconv.FormatBool(status == "ok"),
	})
}

// ensureCleanGitTree aborts the upgrade if projectRoot is inside a git
// working tree that has uncommitted changes. The check is skipped when the
// directory is not a git repository (greenfield projects, vendored copies)
// or when --allow-dirty was passed.
func ensureCleanGitTree(ctx context.Context, projectRoot string, allowDirty bool) error {
	if allowDirty {
		return nil
	}

	dirty, isRepo, err := git.IsWorkingTreeDirty(ctx, projectRoot)
	if err != nil {
		return fmt.Errorf("could not read git working tree status: %w", err)
	}

	if !isRepo || !dirty {
		return nil
	}

	return fmt.Errorf(
		"the upgrade rewrites composer.json and removes recipe-managed files, so the working tree must be clean in %s - "+
			"commit or stash your changes, or rerun with --allow-dirty to override",
		projectRoot,
	)
}

// ensureAllPluginsAreComposerManaged aborts when custom/plugins/ contains
// directories that are not tracked by composer. The upgrade can only bump
// plugin constraints in composer.json, so out-of-band drops would otherwise
// silently keep stale code on disk.
func ensureAllPluginsAreComposerManaged(projectRoot string, allow bool) error {
	if allow {
		return nil
	}

	orphans, err := projectupgrade.FindNonComposerPlugins(projectRoot)
	if err != nil {
		return fmt.Errorf("scan custom/plugins: %w", err)
	}
	if len(orphans) == 0 {
		return nil
	}

	return fmt.Errorf(
		"the upgrade can only bump composer-managed plugins, but %d director(ies) in custom/plugins/ are not tracked by composer:\n  %s\n\nrun `shopware-cli project autofix composer-plugins` to migrate them, or rerun with --allow-non-composer to override",
		len(orphans),
		strings.Join(orphans, "\n  "),
	)
}

func init() {
	projectRootCmd.AddCommand(projectUpgradeCmd)
	projectUpgradeCmd.Flags().String("to", "", "Target Shopware version. Skips the interactive wizard.")
	projectUpgradeCmd.Flags().Bool("allow-dirty", false, "Allow running the upgrade even when the git working tree has uncommitted changes.")
	projectUpgradeCmd.Flags().Bool("allow-non-composer", false, "Allow running the upgrade even when custom/plugins/ contains plugins not managed by composer.")
}
