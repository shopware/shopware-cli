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

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/flexmigrator"
	"github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/projectupgrade"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "Upgrade the Shopware version of this project",
	Long: `Upgrade the Shopware project to a newer version. The command picks an
upgrade target, resolves incompatible custom plugins, rewrites composer.json
for the new version, runs composer update --with-all-dependencies, and then
invokes vendor/bin/shopware-deployment-helper to run system:update:prepare,
migrations, system:update:finish and the rest of the deployment lifecycle.
shopware/deployment-helper is added to composer.json automatically when the
project doesn't require it yet.`,
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
		if err := ensureCleanGitTree(ctx, projectRoot, allowDirty); err != nil {
			return err
		}

		allowNonComposer, _ := cmd.Flags().GetBool("allow-non-composer")
		if err := ensureAllPluginsAreComposerManaged(projectRoot, allowNonComposer); err != nil {
			return err
		}

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

		registry, err := buildRegistry(cmd, projectRoot)
		if err != nil {
			return err
		}

		result, err := projectupgrade.RunWizard(projectupgrade.WizardOptions{
			ProjectRoot:      projectRoot,
			ComposerJSONPath: composerJsonPath,
			CurrentVersion:   currentVersion,
			UpdateVersions:   updateVersions,
			Extensions:       extensions,
			Executor:         cmdExecutor,
			Registry:         registry,
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

	if err := runCompatibilityCheck(ctx, currentVersion, targetVersion); err != nil {
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

	log.Infof("Checking custom plugins for incompatibilities")
	registry, _ := buildRegistry(cmd, projectRoot)
	result, err := projectupgrade.ResolveIncompatiblePlugins(ctx, composerJsonPath, targetVersion, registry)
	if err != nil {
		return fmt.Errorf("resolve incompatible plugins: %w", err)
	}

	if result != nil {
		for _, action := range result.Bumped() {
			log.Infof("Bumped %s: %s → %s", tui.YellowText.Render(action.Name), action.OldConstraint, action.NewConstraint)
		}
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

func runCompatibilityCheck(ctx context.Context, currentVersion *version.Version, targetVersion string) error {
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
			Title("Some installed extensions have no compatible version for the target version").
			Description("They will be removed from composer.json so the upgrade can proceed. Re-require them once they publish a compatible release. Proceed anyway?").
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

	if !git.IsRepository(ctx, projectRoot) {
		return nil
	}

	changes, err := git.WorkingTreeStatus(ctx, projectRoot)
	if err != nil {
		return fmt.Errorf("could not read git working tree status: %w", err)
	}

	if len(changes) == 0 {
		return nil
	}

	preview := changes
	const maxPreview = 10
	suffix := ""
	if len(preview) > maxPreview {
		preview = preview[:maxPreview]
		suffix = fmt.Sprintf("\n  … and %d more", len(changes)-maxPreview)
	}

	return fmt.Errorf(
		"the upgrade rewrites composer.json and removes recipe-managed files, so the working tree must be clean - "+
			"%d uncommitted change(s) detected in %s:\n  %s%s\n\ncommit or stash your changes, or rerun with --allow-dirty to override",
		len(changes),
		projectRoot,
		strings.Join(preview, "\n  "),
		suffix,
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

// buildRegistry constructs the package registry used to look up newer
// compatible plugin versions. The Shopware Packages token is read from the
// SHOPWARE_PACKAGES_TOKEN env var, the project's auth.json, or — in
// interactive mode — prompted from the user if the project has store
// plugins. Missing tokens degrade gracefully: store lookups fall back to the
// "remove plugin" behaviour.
func buildRegistry(cmd *cobra.Command, projectRoot string) (projectupgrade.Registry, error) {
	token := storeTokenFromAuthJSON(projectRoot)

	hasStorePlugins, err := projectHasStorePlugins(projectRoot)
	if err != nil {
		logging.FromContext(cmd.Context()).Debugf("could not inspect installed.json: %v", err)
	}

	if token == "" && hasStorePlugins && system.IsInteractionEnabled(cmd.Context()) {
		var entered string
		if err := huh.NewInput().
			Title("Shopware Packages token (packages.shopware.com)").
			Description("Used to look up newer compatible versions of store plugins. Leave empty to skip store lookups.").
			Value(&entered).
			EchoMode(huh.EchoModePassword).
			Run(); err != nil {
			return nil, err
		}
		token = strings.TrimSpace(entered)
	}

	return projectupgrade.DefaultRegistry(token), nil
}

func storeTokenFromAuthJSON(projectRoot string) string {
	if v := strings.TrimSpace(os.Getenv("SHOPWARE_PACKAGES_TOKEN")); v != "" {
		return v
	}

	authPath := path.Join(projectRoot, "auth.json")
	auth, err := packagist.ReadComposerAuth(authPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(auth.BearerAuth["packages.shopware.com"])
}

func projectHasStorePlugins(projectRoot string) (bool, error) {
	composerJson, err := packagist.ReadComposerJson(path.Join(projectRoot, "composer.json"))
	if err != nil {
		return false, err
	}
	for name := range composerJson.Require {
		if strings.HasPrefix(name, "store.shopware.com/") {
			return true, nil
		}
	}
	return false, nil
}

func init() {
	projectRootCmd.AddCommand(projectUpgradeCmd)
	projectUpgradeCmd.Flags().String("to", "", "Target Shopware version. Skips the interactive wizard.")
	projectUpgradeCmd.Flags().Bool("allow-dirty", false, "Allow running the upgrade even when the git working tree has uncommitted changes.")
	projectUpgradeCmd.Flags().Bool("allow-non-composer", false, "Allow running the upgrade even when custom/plugins/ contains plugins not managed by composer.")
}
