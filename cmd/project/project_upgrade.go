package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/flexmigrator"
	"github.com/shopware/shopware-cli/internal/projectupgrade"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the Shopware version of this project",
	Long: `Upgrade the Shopware project to a newer version. This command mirrors
the behaviour of the shopware/web-installer: it picks an upgrade target,
removes incompatible custom plugins, rewrites composer.json for the new
version, runs composer update --with-all-dependencies, and finally runs
bin/console system:update:prepare and system:update:finish.`,
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
		if !system.IsInteractionEnabled(ctx) || cmd.Flag("to").Value.String() != "" {
			return runUpgradeHeadless(cmd, projectRoot, composerJsonPath, currentVersion, updateVersions, cmdExecutor)
		}

		// Interactive: hand off to the devtui-styled wizard.
		_, extensions, err := getLocalExtensions()
		if err != nil {
			log.Warnf("Could not gather local extensions for compatibility check: %v", err)
			extensions = nil
		}

		target, success, err := projectupgrade.RunWizard(projectupgrade.WizardOptions{
			ProjectRoot:      projectRoot,
			ComposerJSONPath: composerJsonPath,
			CurrentVersion:   currentVersion,
			UpdateVersions:   updateVersions,
			Extensions:       extensions,
			Executor:         cmdExecutor,
		})

		status := "ok"
		switch {
		case errors.Is(err, projectupgrade.ErrCancelled):
			status = "cancelled"
		case err != nil:
			status = "failed"
		case !success:
			status = "failed"
		}

		trackUpgrade(ctx, currentVersion.String(), target, status)

		if errors.Is(err, projectupgrade.ErrCancelled) {
			fmt.Println("Upgrade cancelled.")
			return nil
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

	if err := runCompatibilityCheck(ctx, projectRoot, currentVersion, targetVersion); err != nil {
		return err
	}

	confirmed := !system.IsInteractionEnabled(ctx)
	if !confirmed {
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Upgrade Shopware from %s to %s?", currentVersion.String(), targetVersion)).
			Description("This will modify composer.json, run composer update --with-all-dependencies, and execute system:update:prepare/finish. Commit your changes before running this command.").
			Value(&confirmed).
			Run(); err != nil {
			return err
		}
	}

	if !confirmed {
		return fmt.Errorf("upgrade cancelled")
	}

	log.Infof("Backing up composer.json")
	backup, err := os.ReadFile(composerJsonPath)
	if err != nil {
		return fmt.Errorf("failed to backup composer.json: %w", err)
	}

	log.Infof("Cleaning up stale recipe files")
	if err := flexmigrator.CleanupByHash(projectRoot); err != nil {
		return fmt.Errorf("cleanup stale files: %w", err)
	}

	log.Infof("Checking custom plugins for incompatibilities")
	removed, err := projectupgrade.RemoveIncompatiblePlugins(composerJsonPath, targetVersion)
	if err != nil {
		restoreComposerJson(ctx, composerJsonPath, backup)
		return fmt.Errorf("remove incompatible plugins: %w", err)
	}

	for _, name := range removed {
		log.Infof("Removed incompatible plugin %s from composer.json. Re-require it once a compatible version is published.", tui.YellowText.Render(name))
	}

	log.Infof("Updating composer.json to %s", targetVersion)
	if err := projectupgrade.UpdateComposerJson(composerJsonPath, targetVersion); err != nil {
		restoreComposerJson(ctx, composerJsonPath, backup)
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
		restoreComposerJson(ctx, composerJsonPath, backup)
		trackUpgrade(ctx, currentVersion.String(), targetVersion, "composer_update_failed")
		return fmt.Errorf("composer update failed, composer.json was restored: %w", err)
	}

	log.Infof("Running bin/console system:update:prepare")
	prepareCmd := cmdExecutor.ConsoleCommand(ctx, "system:update:prepare", "--no-interaction")
	prepareCmd.Cmd.Stdin = cmd.InOrStdin()
	prepareCmd.Cmd.Stdout = cmd.OutOrStdout()
	prepareCmd.Cmd.Stderr = cmd.ErrOrStderr()

	if err := prepareCmd.Run(); err != nil {
		trackUpgrade(ctx, currentVersion.String(), targetVersion, "system_update_prepare_failed")
		return fmt.Errorf("system:update:prepare failed: %w", err)
	}

	log.Infof("Running bin/console system:update:finish")
	finishCmd := cmdExecutor.ConsoleCommand(ctx, "system:update:finish", "--no-interaction")
	finishCmd.Cmd.Stdin = cmd.InOrStdin()
	finishCmd.Cmd.Stdout = cmd.OutOrStdout()
	finishCmd.Cmd.Stderr = cmd.ErrOrStderr()

	if err := finishCmd.Run(); err != nil {
		trackUpgrade(ctx, currentVersion.String(), targetVersion, "system_update_finish_failed")
		return fmt.Errorf("system:update:finish failed: %w", err)
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

func runCompatibilityCheck(ctx context.Context, projectRoot string, currentVersion *version.Version, targetVersion string) error {
	log := logging.FromContext(ctx)

	_, extensions, err := getLocalExtensions()
	if err != nil {
		log.Warnf("Skipping extension compatibility check: %v", err)
		return nil
	}

	if len(extensions) == 0 {
		return nil
	}

	requests := make([]account_api.UpdateCheckExtension, 0, len(extensions))
	for name, v := range extensions {
		requests = append(requests, account_api.UpdateCheckExtension{Name: name, Version: v})
	}

	updates, err := account_api.GetFutureExtensionUpdates(ctx, currentVersion.String(), targetVersion, requests)
	if err != nil {
		log.Warnf("Skipping extension compatibility check: %v", err)
		return nil
	}

	for _, name := range requests {
		found := false
		for _, update := range updates {
			if update.Name == name.Name {
				found = true
				break
			}
		}

		if !found {
			updates = append(updates, account_api.UpdateCheckExtensionCompatibility{
				Name: name.Name,
				Status: account_api.UpdateCheckExtensionCompatibilityStatus{
					Label: "Not available in Store",
				},
			})
		}
	}

	t := table.New().Border(lipgloss.NormalBorder()).Headers("Extension Name", "Compatible")
	for _, update := range updates {
		t.Row(update.Name, update.Status.Label)
	}
	fmt.Println(t.Render())

	hasBlockers := false
	for _, update := range updates {
		if update.Status.IsBlocker() {
			hasBlockers = true
			break
		}
	}

	if hasBlockers && system.IsInteractionEnabled(ctx) {
		var proceed bool
		if err := huh.NewConfirm().
			Title("Some installed extensions are not yet compatible with the target version").
			Description("Continuing may break those extensions. Proceed anyway?").
			Value(&proceed).
			Run(); err != nil {
			return err
		}

		if !proceed {
			return fmt.Errorf("upgrade cancelled due to incompatible extensions")
		}
	}

	return nil
}

func restoreComposerJson(ctx context.Context, composerJsonPath string, backup []byte) {
	if err := os.WriteFile(composerJsonPath, backup, 0o644); err != nil {
		logging.FromContext(ctx).Errorf("failed to restore composer.json from backup: %v", err)
	}
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

func init() {
	projectRootCmd.AddCommand(projectUpgradeCmd)
	projectUpgradeCmd.Flags().String("to", "", "Target Shopware version. Skips the interactive wizard.")
}
