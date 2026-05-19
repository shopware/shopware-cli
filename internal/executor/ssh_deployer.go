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
//
// When multiple hosts are configured, every step of the deploy is executed
// against each host sequentially with the same release name, so all hosts
// converge on the same release.
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
	hosts := d.sshCfg.ResolvedHosts()

	log.Infof("Deploying release %s to %d host(s)", release, len(hosts))

	for _, h := range hosts {
		if err := d.deployHost(ctx, h, release); err != nil {
			return fmt.Errorf("host %s: %w", h.Host, err)
		}
	}

	log.Infof("Release %s is now active on all hosts", release)
	return nil
}

func (d *SSHDeployer) deployHost(ctx context.Context, h shop.EnvironmentSSHHostConfig, release string) error {
	log := logging.FromContext(ctx)
	releaseDir := path.Join(h.DeployPath, "releases", release)
	sharedDir := path.Join(h.DeployPath, "shared")
	currentLink := path.Join(h.DeployPath, "current")

	log.Infof("[%s] Deploying release %s", h.Host, release)

	if err := d.runRemote(ctx, h, fmt.Sprintf("mkdir -p %s %s", shellQuote(path.Join(h.DeployPath, "releases")), shellQuote(sharedDir))); err != nil {
		return fmt.Errorf("preparing remote layout: %w", err)
	}

	if err := d.runHook(ctx, h, "pre", d.deploymentHooks().Pre, currentLink); err != nil {
		return err
	}

	log.Infof("[%s] Uploading source via rsync", h.Host)
	if err := d.rsyncTo(ctx, h, releaseDir); err != nil {
		return fmt.Errorf("rsync upload: %w", err)
	}

	if err := d.linkShared(ctx, h, releaseDir, sharedDir); err != nil {
		return fmt.Errorf("linking shared paths: %w", err)
	}

	isUpdate, err := d.currentExists(ctx, h)
	if err != nil {
		return err
	}

	composerCmd := "composer install --no-dev --optimize-autoloader --no-interaction"
	if err := d.runRemote(ctx, h, fmt.Sprintf("cd %s && %s", shellQuote(releaseDir), composerCmd)); err != nil {
		return fmt.Errorf("composer install on remote: %w", err)
	}

	hooks := d.deploymentHooks()
	if isUpdate {
		if err := d.runHook(ctx, h, "pre-update", hooks.PreUpdate, releaseDir); err != nil {
			return err
		}
	} else {
		if err := d.runHook(ctx, h, "pre-install", hooks.PreInstall, releaseDir); err != nil {
			return err
		}
	}

	if err := d.runDeploymentHelper(ctx, h, releaseDir); err != nil {
		return fmt.Errorf("shopware-deployment-helper: %w", err)
	}

	if err := d.runOneTimeTasks(ctx, h, releaseDir, sharedDir); err != nil {
		return fmt.Errorf("one-time tasks: %w", err)
	}

	if isUpdate {
		if err := d.runHook(ctx, h, "post-update", hooks.PostUpdate, releaseDir); err != nil {
			return err
		}
	} else {
		if err := d.runHook(ctx, h, "post-install", hooks.PostInstall, releaseDir); err != nil {
			return err
		}
	}

	log.Infof("[%s] Activating release %s", h.Host, release)
	if err := d.swapCurrent(ctx, h, releaseDir, currentLink); err != nil {
		return fmt.Errorf("activating release: %w", err)
	}

	if err := d.runHook(ctx, h, "post", hooks.Post, currentLink); err != nil {
		return err
	}

	if err := d.cleanupOldReleases(ctx, h); err != nil {
		log.Warnf("[%s] cleaning up old releases failed: %v", h.Host, err)
	}

	return nil
}

func (d *SSHDeployer) ListReleases(ctx context.Context) ([]Release, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}

	hosts := d.sshCfg.ResolvedHosts()
	primary := hosts[0]

	out, err := d.captureRemote(ctx, primary, fmt.Sprintf("ls -1 %s 2>/dev/null || true", shellQuote(path.Join(primary.DeployPath, "releases"))))
	if err != nil {
		return nil, fmt.Errorf("listing releases: %w", err)
	}

	current, _ := d.currentRelease(ctx, primary)

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
		return fmt.Errorf("no releases found on primary host")
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
			return fmt.Errorf("release %q not found on primary host", target)
		}
	}

	log := logging.FromContext(ctx)
	log.Infof("Rolling back to release %s on all hosts", target)

	for _, h := range d.sshCfg.ResolvedHosts() {
		log.Infof("[%s] Activating release %s", h.Host, target)
		releaseDir := path.Join(h.DeployPath, "releases", target)
		currentLink := path.Join(h.DeployPath, "current")

		if err := d.swapCurrent(ctx, h, releaseDir, currentLink); err != nil {
			return fmt.Errorf("[%s] activating release %s: %w", h.Host, target, err)
		}

		if d.shouldClearCache() {
			if err := d.runRemote(ctx, h, fmt.Sprintf("cd %s && php bin/console cache:clear", shellQuote(currentLink))); err != nil {
				log.Warnf("[%s] cache:clear after rollback failed: %v", h.Host, err)
			}
		}
	}

	log.Infof("Rolled back to %s", target)
	return nil
}

func (d *SSHDeployer) validate() error {
	if d.sshCfg == nil {
		return fmt.Errorf("ssh configuration missing for environment")
	}
	hosts := d.sshCfg.ResolvedHosts()
	if len(hosts) == 0 {
		return fmt.Errorf("ssh.host or ssh.hosts is required")
	}
	for _, h := range hosts {
		if h.Host == "" {
			return fmt.Errorf("ssh.hosts entry is missing host")
		}
		if h.DeployPath == "" {
			return fmt.Errorf("ssh.deploy_path is required (host %s)", h.Host)
		}
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

// runRemote streams a shell command to the given host and returns an error
// if it exits non-zero. Output is forwarded to stdout/stderr.
func (d *SSHDeployer) runRemote(ctx context.Context, h shop.EnvironmentSSHHostConfig, command string) error {
	cmd := d.sshCommand(ctx, h, command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// captureRemote runs a remote shell command and returns its stdout.
func (d *SSHDeployer) captureRemote(ctx context.Context, h shop.EnvironmentSSHHostConfig, command string) ([]byte, error) {
	cmd := d.sshCommand(ctx, h, command)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (d *SSHDeployer) sshCommand(ctx context.Context, h shop.EnvironmentSSHHostConfig, command string) *exec.Cmd {
	args := append(sshArgsFor(h), command)
	cmd := exec.CommandContext(ctx, "ssh", args...)
	logCmd(ctx, cmd)
	return cmd
}

func (d *SSHDeployer) rsyncTo(ctx context.Context, h shop.EnvironmentSSHHostConfig, releaseDir string) error {
	if err := d.runRemote(ctx, h, fmt.Sprintf("mkdir -p %s", shellQuote(releaseDir))); err != nil {
		return err
	}

	target := sshTargetFor(h) + ":" + releaseDir + "/"

	sshCmd := "ssh -o BatchMode=yes"
	if h.Port != 0 && h.Port != 22 {
		sshCmd += " -p " + strconv.Itoa(h.Port)
	}
	if h.IdentityFile != "" {
		sshCmd += " -i " + shellQuote(expandHome(h.IdentityFile))
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

func (d *SSHDeployer) linkShared(ctx context.Context, h shop.EnvironmentSSHHostConfig, releaseDir, sharedDir string) error {
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
	return d.runRemote(ctx, h, script.String())
}

func (d *SSHDeployer) currentExists(ctx context.Context, h shop.EnvironmentSSHHostConfig) (bool, error) {
	out, err := d.captureRemote(ctx, h, fmt.Sprintf("if [ -L %s ]; then echo yes; else echo no; fi", shellQuote(path.Join(h.DeployPath, "current"))))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(out)) == "yes", nil
}

func (d *SSHDeployer) currentRelease(ctx context.Context, h shop.EnvironmentSSHHostConfig) (string, error) {
	out, err := d.captureRemote(ctx, h, fmt.Sprintf("readlink %s 2>/dev/null || true", shellQuote(path.Join(h.DeployPath, "current"))))
	if err != nil {
		return "", err
	}
	target := strings.TrimSpace(string(out))
	if target == "" {
		return "", nil
	}
	return path.Base(target), nil
}

func (d *SSHDeployer) swapCurrent(ctx context.Context, h shop.EnvironmentSSHHostConfig, releaseDir, currentLink string) error {
	return d.runRemote(ctx, h, fmt.Sprintf("ln -sfn %s %s", shellQuote(releaseDir), shellQuote(currentLink)))
}

func (d *SSHDeployer) runHook(ctx context.Context, h shop.EnvironmentSSHHostConfig, name, command, workdir string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	logging.FromContext(ctx).Infof("[%s] Running %s hook", h.Host, name)
	return d.runRemote(ctx, h, fmt.Sprintf("cd %s && %s", shellQuote(workdir), command))
}

func (d *SSHDeployer) runDeploymentHelper(ctx context.Context, h shop.EnvironmentSSHHostConfig, releaseDir string) error {
	logging.FromContext(ctx).Infof("[%s] Running shopware-deployment-helper", h.Host)
	cmd := fmt.Sprintf("cd %s && if [ -x vendor/bin/shopware-deployment-helper ]; then vendor/bin/shopware-deployment-helper run; else php bin/console system:update:finish --no-interaction && php bin/console cache:clear --no-interaction; fi",
		shellQuote(releaseDir))
	return d.runRemote(ctx, h, cmd)
}

func (d *SSHDeployer) runOneTimeTasks(ctx context.Context, h shop.EnvironmentSSHHostConfig, releaseDir, sharedDir string) error {
	if d.shopCfg == nil || d.shopCfg.ConfigDeployment == nil || len(d.shopCfg.ConfigDeployment.OneTimeTasks) == 0 {
		return nil
	}

	stateFile := path.Join(sharedDir, ".shopware-cli-one-time-tasks")
	if err := d.runRemote(ctx, h, fmt.Sprintf("touch %s", shellQuote(stateFile))); err != nil {
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
		if err := d.runRemote(ctx, h, script); err != nil {
			return fmt.Errorf("one-time task %s: %w", task.Id, err)
		}
	}
	return nil
}

func (d *SSHDeployer) cleanupOldReleases(ctx context.Context, h shop.EnvironmentSSHHostConfig) error {
	keep := d.sshCfg.KeepReleases
	if keep <= 0 {
		keep = 5
	}

	out, err := d.captureRemote(ctx, h, fmt.Sprintf("ls -1 %s 2>/dev/null || true", shellQuote(path.Join(h.DeployPath, "releases"))))
	if err != nil {
		return err
	}
	currentName, _ := d.currentRelease(ctx, h)

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) <= keep {
		return nil
	}

	toRemove := names[:len(names)-keep]
	for _, name := range toRemove {
		if name == currentName {
			continue
		}
		releasePath := path.Join(h.DeployPath, "releases", name)
		if err := d.runRemote(ctx, h, fmt.Sprintf("rm -rf %s", shellQuote(releasePath))); err != nil {
			return err
		}
	}
	return nil
}
