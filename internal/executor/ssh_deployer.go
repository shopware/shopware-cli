package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

const deployTimeFormat = "20060102150405"

// SSHDeployer is a release-based, Deployer-style implementation that uses
// rsync to transfer the local working directory and ssh to run remote
// commands. It manages atomic releases via a `current` symlink and a
// `releases/` directory, plus persistent `shared/` files and directories.
type SSHDeployer struct {
	exec        *SSHExecutor
	projectRoot string
	shopCfg     *shop.Config
	envCfg      *shop.EnvironmentConfig
	sshCfg      *shop.EnvironmentSSHConfig
}

func (d *SSHDeployer) Deploy(ctx context.Context) error {
	if err := d.validate(); err != nil {
		return err
	}

	log := logging.FromContext(ctx)
	release := time.Now().UTC().Format(deployTimeFormat)
	releaseDir := path.Join(d.sshCfg.DeployPath, "releases", release)
	sharedDir := path.Join(d.sshCfg.DeployPath, "shared")
	currentLink := path.Join(d.sshCfg.DeployPath, "current")

	log.Infof("Deploying release %s to %s", release, d.exec.sshTarget())

	if err := d.runRemote(ctx, fmt.Sprintf("mkdir -p %s %s", shellQuote(path.Join(d.sshCfg.DeployPath, "releases")), shellQuote(sharedDir))); err != nil {
		return fmt.Errorf("preparing remote layout: %w", err)
	}

	if err := d.runHook(ctx, "pre", d.deploymentHooks().Pre, currentLink); err != nil {
		return err
	}

	log.Infof("Uploading source via rsync")
	if err := d.rsyncTo(ctx, releaseDir); err != nil {
		return fmt.Errorf("rsync upload: %w", err)
	}

	if err := d.linkShared(ctx, releaseDir, sharedDir); err != nil {
		return fmt.Errorf("linking shared paths: %w", err)
	}

	isUpdate, err := d.currentExists(ctx)
	if err != nil {
		return err
	}

	composerCmd := "composer install --no-dev --optimize-autoloader --no-interaction"
	if err := d.runRemote(ctx, fmt.Sprintf("cd %s && %s", shellQuote(releaseDir), composerCmd)); err != nil {
		return fmt.Errorf("composer install on remote: %w", err)
	}

	hooks := d.deploymentHooks()
	if isUpdate {
		if err := d.runHook(ctx, "pre-update", hooks.PreUpdate, releaseDir); err != nil {
			return err
		}
	} else {
		if err := d.runHook(ctx, "pre-install", hooks.PreInstall, releaseDir); err != nil {
			return err
		}
	}

	if err := d.runDeploymentHelper(ctx, releaseDir); err != nil {
		return fmt.Errorf("shopware-deployment-helper: %w", err)
	}

	if err := d.runOneTimeTasks(ctx, releaseDir, sharedDir); err != nil {
		return fmt.Errorf("one-time tasks: %w", err)
	}

	if isUpdate {
		if err := d.runHook(ctx, "post-update", hooks.PostUpdate, releaseDir); err != nil {
			return err
		}
	} else {
		if err := d.runHook(ctx, "post-install", hooks.PostInstall, releaseDir); err != nil {
			return err
		}
	}

	log.Infof("Activating release %s", release)
	if err := d.swapCurrent(ctx, releaseDir, currentLink); err != nil {
		return fmt.Errorf("activating release: %w", err)
	}

	if err := d.runHook(ctx, "post", hooks.Post, currentLink); err != nil {
		return err
	}

	if err := d.cleanupOldReleases(ctx); err != nil {
		log.Warnf("cleaning up old releases failed: %v", err)
	}

	log.Infof("Release %s is now active", release)
	return nil
}

func (d *SSHDeployer) ListReleases(ctx context.Context) ([]Release, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}

	out, err := d.captureRemote(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null || true", shellQuote(path.Join(d.sshCfg.DeployPath, "releases"))))
	if err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}

	current, _ := d.currentRelease(ctx)

	var releases []Release
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		r := Release{Name: name, Current: name == current}
		if t, err := time.Parse(deployTimeFormat, name); err == nil {
			r.CreatedAt = t
		}
		releases = append(releases, r)
	}

	sort.Slice(releases, func(i, j int) bool {
		return releases[i].Name < releases[j].Name
	})
	return releases, nil
}

func (d *SSHDeployer) Rollback(ctx context.Context, name string) error {
	if err := d.validate(); err != nil {
		return err
	}

	releases, err := d.ListReleases(ctx)
	if err != nil {
		return err
	}
	if len(releases) == 0 {
		return fmt.Errorf("no releases found on %s", d.exec.sshTarget())
	}

	target := name
	if target == "" {
		currentIdx := -1
		for i, r := range releases {
			if r.Current {
				currentIdx = i
				break
			}
		}
		if currentIdx <= 0 {
			return fmt.Errorf("no previous release available to roll back to")
		}
		target = releases[currentIdx-1].Name
	} else {
		idx := slices.IndexFunc(releases, func(r Release) bool { return r.Name == target })
		if idx == -1 {
			return fmt.Errorf("release %q not found on %s", target, d.exec.sshTarget())
		}
	}

	log := logging.FromContext(ctx)
	log.Infof("Rolling back to release %s", target)

	releaseDir := path.Join(d.sshCfg.DeployPath, "releases", target)
	currentLink := path.Join(d.sshCfg.DeployPath, "current")

	if err := d.swapCurrent(ctx, releaseDir, currentLink); err != nil {
		return fmt.Errorf("activating release %s: %w", target, err)
	}

	if d.shouldClearCache() {
		if err := d.runRemote(ctx, fmt.Sprintf("cd %s && php bin/console cache:clear", shellQuote(currentLink))); err != nil {
			log.Warnf("cache:clear after rollback failed: %v", err)
		}
	}

	log.Infof("Rolled back to %s", target)
	return nil
}

func (d *SSHDeployer) validate() error {
	if d.sshCfg == nil {
		return fmt.Errorf("ssh configuration missing for environment")
	}
	if d.sshCfg.Host == "" {
		return fmt.Errorf("ssh.host is required")
	}
	if d.sshCfg.DeployPath == "" {
		return fmt.Errorf("ssh.deploy_path is required")
	}
	if d.projectRoot == "" {
		return fmt.Errorf("project root not set")
	}
	return nil
}

func (d *SSHDeployer) deploymentHooks() shop.ConfigDeploymentHooks {
	if d.shopCfg == nil || d.shopCfg.ConfigDeployment == nil {
		return shop.ConfigDeploymentHooks{}
	}
	return d.shopCfg.ConfigDeployment.Hooks
}

func (d *SSHDeployer) shouldClearCache() bool {
	if d.shopCfg == nil || d.shopCfg.ConfigDeployment == nil {
		return true
	}
	return d.shopCfg.ConfigDeployment.Cache.AlwaysClear
}

// runRemote streams a shell command to the remote and returns an error if it
// exits non-zero. Output is forwarded to the calling process's stdout/stderr.
func (d *SSHDeployer) runRemote(ctx context.Context, command string) error {
	cmd := d.sshCommand(ctx, command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// captureRemote runs a remote shell command and returns its stdout. Stderr is
// forwarded so connection issues are visible.
func (d *SSHDeployer) captureRemote(ctx context.Context, command string) ([]byte, error) {
	cmd := d.sshCommand(ctx, command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *SSHDeployer) sshCommand(ctx context.Context, command string) *exec.Cmd {
	args := append(d.exec.sshArgs(), command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	logCmd(ctx, cmd)
	return cmd
}

func (d *SSHDeployer) rsyncTo(ctx context.Context, releaseDir string) error {
	if err := d.runRemote(ctx, fmt.Sprintf("mkdir -p %s", shellQuote(releaseDir))); err != nil {
		return err
	}

	target := d.exec.sshTarget() + ":" + releaseDir + "/"

	sshCmd := "ssh -o BatchMode=yes"
	if d.sshCfg.Port != 0 && d.sshCfg.Port != 22 {
		sshCmd += " -p " + strconv.Itoa(d.sshCfg.Port)
	}
	if d.sshCfg.IdentityFile != "" {
		sshCmd += " -i " + shellQuote(expandHome(d.sshCfg.IdentityFile))
	}

	args := []string{"-az", "--delete", "-e", sshCmd}
	for _, ex := range defaultRsyncExcludes() {
		args = append(args, "--exclude", ex)
	}
	for _, ex := range d.sshCfg.Excludes {
		args = append(args, "--exclude", ex)
	}
	args = append(args, d.sshCfg.RsyncOptions...)

	source := d.projectRoot
	if !strings.HasSuffix(source, "/") {
		source += "/"
	}
	args = append(args, source, target)

	cmd := exec.CommandContext(ctx, "rsync", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	logCmd(ctx, cmd)
	return cmd.Run()
}

func defaultRsyncExcludes() []string {
	return []string{
		".git/",
		"node_modules/",
		"var/cache/",
		"var/log/",
		".env.local",
		".shopware-project.local.yml",
	}
}

func (d *SSHDeployer) linkShared(ctx context.Context, releaseDir, sharedDir string) error {
	var script strings.Builder
	script.WriteString("set -e\n")

	for _, rel := range d.sshCfg.SharedDirs {
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		sharedPath := path.Join(sharedDir, rel)
		releasePath := path.Join(releaseDir, rel)
		fmt.Fprintf(&script, "mkdir -p %s\n", shellQuote(sharedPath))
		fmt.Fprintf(&script, "mkdir -p %s\n", shellQuote(path.Dir(releasePath)))
		fmt.Fprintf(&script, "rm -rf %s\n", shellQuote(releasePath))
		fmt.Fprintf(&script, "ln -sfn %s %s\n", shellQuote(sharedPath), shellQuote(releasePath))
	}

	for _, rel := range d.sshCfg.SharedFiles {
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			continue
		}
		sharedPath := path.Join(sharedDir, rel)
		releasePath := path.Join(releaseDir, rel)
		fmt.Fprintf(&script, "mkdir -p %s\n", shellQuote(path.Dir(sharedPath)))
		fmt.Fprintf(&script, "if [ ! -f %s ]; then if [ -f %s ]; then cp %s %s; else touch %s; fi; fi\n",
			shellQuote(sharedPath), shellQuote(releasePath),
			shellQuote(releasePath), shellQuote(sharedPath),
			shellQuote(sharedPath))
		fmt.Fprintf(&script, "mkdir -p %s\n", shellQuote(path.Dir(releasePath)))
		fmt.Fprintf(&script, "rm -f %s\n", shellQuote(releasePath))
		fmt.Fprintf(&script, "ln -sfn %s %s\n", shellQuote(sharedPath), shellQuote(releasePath))
	}

	if script.Len() == 0 {
		return nil
	}
	return d.runRemote(ctx, script.String())
}

func (d *SSHDeployer) currentExists(ctx context.Context) (bool, error) {
	out, err := d.captureRemote(ctx, fmt.Sprintf("if [ -L %s ]; then echo yes; else echo no; fi", shellQuote(path.Join(d.sshCfg.DeployPath, "current"))))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "yes", nil
}

func (d *SSHDeployer) currentRelease(ctx context.Context) (string, error) {
	out, err := d.captureRemote(ctx, fmt.Sprintf("readlink %s 2>/dev/null || true", shellQuote(path.Join(d.sshCfg.DeployPath, "current"))))
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(string(out))
	if target == "" {
		return "", nil
	}
	return path.Base(target), nil
}

func (d *SSHDeployer) swapCurrent(ctx context.Context, releaseDir, currentLink string) error {
	return d.runRemote(ctx, fmt.Sprintf("ln -sfn %s %s", shellQuote(releaseDir), shellQuote(currentLink)))
}

func (d *SSHDeployer) runHook(ctx context.Context, name, command, workdir string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	logging.FromContext(ctx).Infof("Running %s hook", name)
	return d.runRemote(ctx, fmt.Sprintf("cd %s && %s", shellQuote(workdir), command))
}

func (d *SSHDeployer) runDeploymentHelper(ctx context.Context, releaseDir string) error {
	logging.FromContext(ctx).Infof("Running shopware-deployment-helper")
	cmd := fmt.Sprintf("cd %s && if [ -x vendor/bin/shopware-deployment-helper ]; then vendor/bin/shopware-deployment-helper run; else php bin/console system:update:finish --no-interaction && php bin/console cache:clear --no-interaction; fi",
		shellQuote(releaseDir))
	return d.runRemote(ctx, cmd)
}

func (d *SSHDeployer) runOneTimeTasks(ctx context.Context, releaseDir, sharedDir string) error {
	if d.shopCfg == nil || d.shopCfg.ConfigDeployment == nil || len(d.shopCfg.ConfigDeployment.OneTimeTasks) == 0 {
		return nil
	}

	stateFile := path.Join(sharedDir, ".shopware-cli-one-time-tasks")
	// Ensure state file exists for the grep below.
	if err := d.runRemote(ctx, fmt.Sprintf("touch %s", shellQuote(stateFile))); err != nil {
		return err
	}

	for _, task := range d.shopCfg.ConfigDeployment.OneTimeTasks {
		idLine := task.Id
		script := fmt.Sprintf(
			"if grep -Fxq %s %s; then echo 'skipping one-time task %s'; else cd %s && %s && echo %s >> %s; fi",
			shellQuote(idLine), shellQuote(stateFile), task.Id,
			shellQuote(releaseDir), task.Script,
			shellQuote(idLine), shellQuote(stateFile),
		)
		if err := d.runRemote(ctx, script); err != nil {
			return fmt.Errorf("one-time task %s: %w", task.Id, err)
		}
	}
	return nil
}

func (d *SSHDeployer) cleanupOldReleases(ctx context.Context) error {
	keep := d.sshCfg.KeepReleases
	if keep <= 0 {
		keep = 5
	}

	releases, err := d.ListReleases(ctx)
	if err != nil {
		return err
	}
	if len(releases) <= keep {
		return nil
	}

	toRemove := releases[:len(releases)-keep]
	for _, r := range toRemove {
		if r.Current {
			continue
		}
		releasePath := path.Join(d.sshCfg.DeployPath, "releases", r.Name)
		if err := d.runRemote(ctx, fmt.Sprintf("rm -rf %s", shellQuote(releasePath))); err != nil {
			return err
		}
	}
	return nil
}
