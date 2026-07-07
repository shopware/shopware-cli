package deployment

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/shopware/shopware-cli/internal/shell"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

const badReleaseMarker = "BAD_RELEASE"

var (
	defaultSharedFiles = []string{".env", "install.lock"}
	defaultSharedDirs  = []string{"files", "public/media", "public/thumbnail", "public/sitemap", "var/log"}
)

const defaultKeepReleases = 5

// deployHost is a single server of the deployment target.
type deployHost struct {
	name string
	conn Connection
}

// sshDeployer deploys a project to one or more hosts using a releases/shared
// directory layout and an atomically switched "current" symlink, similar to
// what Deployer (deployer.org) does.
type sshDeployer struct {
	projectRoot string
	deployPath  string
	config      *shop.EnvironmentDeployment
	hosts       []deployHost

	// injected for tests
	now      func() time.Time
	runLocal func(ctx context.Context, dir string, command string) error
}

func newSSHDeployer(projectRoot string, env *shop.EnvironmentConfig, _ *shop.Config) (Deployer, error) {
	if env.Deployment == nil || env.Deployment.Path == "" {
		return nil, fmt.Errorf("the environment is missing the deployment.path setting")
	}

	if env.SSH == nil || env.SSH.Host == "" {
		return nil, fmt.Errorf("the environment is missing the ssh.host setting")
	}

	var hosts []deployHost

	for _, hostCfg := range env.SSH.AllHosts() {
		conn, err := newSSHConnection(&hostCfg)
		if err != nil {
			for _, host := range hosts {
				_ = host.conn.Close()
			}

			return nil, fmt.Errorf("host %s: %w", hostCfg.Host, err)
		}

		hosts = append(hosts, deployHost{name: hostCfg.Host, conn: conn})
	}

	return &sshDeployer{
		projectRoot: projectRoot,
		deployPath:  strings.TrimRight(env.Deployment.Path, "/"),
		config:      env.Deployment,
		hosts:       hosts,
		now:         time.Now,
		runLocal:    runLocalCommand,
	}, nil
}

func runLocalCommand(ctx context.Context, dir string, command string) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// logPrefix identifies the host in log output for multi-host targets.
func (d *sshDeployer) logPrefix(host deployHost) string {
	if len(d.hosts) == 1 {
		return ""
	}

	return "[" + host.name + "] "
}

func (d *sshDeployer) sharedFiles() []string {
	if d.config.SharedFiles != nil {
		return d.config.SharedFiles
	}

	return defaultSharedFiles
}

func (d *sshDeployer) sharedDirs() []string {
	if d.config.SharedDirs != nil {
		return d.config.SharedDirs
	}

	return defaultSharedDirs
}

func (d *sshDeployer) keepReleases() int {
	if d.config.KeepReleases > 0 {
		return d.config.KeepReleases
	}

	return defaultKeepReleases
}

func (d *sshDeployer) releasesPath() string {
	return path.Join(d.deployPath, "releases")
}

func (d *sshDeployer) sharedPath() string {
	return path.Join(d.deployPath, "shared")
}

func (d *sshDeployer) currentPath() string {
	return path.Join(d.deployPath, "current")
}

func (d *sshDeployer) Deploy(ctx context.Context, opts Options) error {
	logger := logging.FromContext(ctx)

	if !opts.SkipBuildHooks {
		for _, hook := range d.config.Hooks.Build {
			logger.Infof("Running local build hook: %s", hook)

			if err := d.runLocal(ctx, d.projectRoot, hook); err != nil {
				return fmt.Errorf("local build hook %q failed: %w", hook, err)
			}
		}
	}

	releaseName := d.now().UTC().Format("20060102150405")
	releaseDir := path.Join(d.releasesPath(), releaseName)

	// upload to all hosts in parallel, nothing is switched yet so a failing
	// host aborts the deployment without affecting the running release
	uploads, uploadCtx := errgroup.WithContext(ctx)

	for _, host := range d.hosts {
		uploads.Go(func() error {
			if err := d.prepareHost(uploadCtx, host, releaseName, releaseDir); err != nil {
				return fmt.Errorf("host %s: %w", host.name, err)
			}

			return nil
		})
	}

	if err := uploads.Wait(); err != nil {
		return err
	}

	// hooks run host after host so database work (e.g. migrations run by the
	// deployment helper) never executes concurrently
	for _, host := range d.hosts {
		if err := d.runPreSwitchHooks(ctx, host, releaseDir); err != nil {
			return fmt.Errorf("host %s: %w", host.name, err)
		}
	}

	// every host finished its setup, now switch them all
	for _, host := range d.hosts {
		logger.Infof("%sSwitching %s to release %s", d.logPrefix(host), d.currentPath(), releaseName)

		if err := d.switchCurrent(ctx, host, releaseDir); err != nil {
			return fmt.Errorf("host %s: %w", host.name, err)
		}
	}

	for _, host := range d.hosts {
		if err := d.runHooks(ctx, host, d.config.Hooks.PostSwitch, d.currentPath()); err != nil {
			return fmt.Errorf("host %s: %w", host.name, err)
		}
	}

	for _, host := range d.hosts {
		if err := d.cleanupReleases(ctx, host); err != nil {
			logger.Warnf("%sCleanup of old releases failed: %v", d.logPrefix(host), err)
		}
	}

	logger.Infof("Deployed release %s", releaseName)

	return nil
}

// prepareHost creates the release on a single host: directory structure,
// upload and shared symlinks.
func (d *sshDeployer) prepareHost(ctx context.Context, host deployHost, releaseName, releaseDir string) error {
	logger := logging.FromContext(ctx)

	logger.Infof("%sPreparing deployment structure in %s", d.logPrefix(host), d.deployPath)

	if _, err := host.conn.Run(ctx, fmt.Sprintf("mkdir -p %s %s", shell.Quote(d.releasesPath()), shell.Quote(d.sharedPath()))); err != nil {
		return err
	}

	if _, err := host.conn.Run(ctx, fmt.Sprintf("[ ! -e %[1]s ] || [ -L %[1]s ]", shell.Quote(d.currentPath()))); err != nil {
		return fmt.Errorf("%s exists but is not a symlink, refusing to deploy into an unmanaged directory", d.currentPath())
	}

	if _, err := host.conn.Run(ctx, fmt.Sprintf("mkdir -p %s", shell.Quote(releaseDir))); err != nil {
		return err
	}

	logger.Infof("%sUploading project to release %s", d.logPrefix(host), releaseName)

	if err := d.upload(ctx, host, releaseDir); err != nil {
		return err
	}

	logger.Infof("%sLinking shared files and directories", d.logPrefix(host))

	return d.linkShared(ctx, host, releaseDir)
}

func (d *sshDeployer) upload(ctx context.Context, host deployHost, releaseDir string) error {
	exclude := slices.Concat(d.config.Exclude, d.sharedDirs(), d.sharedFiles())

	reader, writer := io.Pipe()

	go func() {
		writer.CloseWithError(writeProjectArchive(writer, d.projectRoot, exclude))
	}()

	if err := host.conn.Stream(ctx, fmt.Sprintf("tar -xzpf - -C %s", shell.Quote(releaseDir)), reader); err != nil {
		_ = reader.CloseWithError(err)
		return fmt.Errorf("upload failed: %w", err)
	}

	// var/cache and var/log are excluded from the upload but Shopware expects them
	_, err := host.conn.Run(ctx, fmt.Sprintf("mkdir -p %s %s", shell.Quote(path.Join(releaseDir, "var", "cache")), shell.Quote(path.Join(releaseDir, "var", "log"))))

	return err
}

func (d *sshDeployer) linkShared(ctx context.Context, host deployHost, releaseDir string) error {
	for _, dir := range d.sharedDirs() {
		dir = strings.Trim(dir, "/")
		sharedTarget := path.Join(d.sharedPath(), dir)
		releasePath := path.Join(releaseDir, dir)

		// seed the shared directory from the release on first deployment (best
		// effort), afterwards drop the uploaded variant and replace it with a
		// symlink into shared/
		script := fmt.Sprintf(
			"if [ -d %[2]s ] && [ ! -e %[1]s ]; then mkdir -p %[3]s && { mv %[2]s %[1]s || true; }; fi && mkdir -p %[1]s && rm -rf %[2]s && mkdir -p %[4]s && ln -sfn %[1]s %[2]s",
			shell.Quote(sharedTarget),
			shell.Quote(releasePath),
			shell.Quote(path.Dir(sharedTarget)),
			shell.Quote(path.Dir(releasePath)),
		)

		if _, err := host.conn.Run(ctx, script); err != nil {
			return fmt.Errorf("cannot link shared directory %s: %w", dir, err)
		}
	}

	for _, file := range d.sharedFiles() {
		file = strings.Trim(file, "/")
		sharedTarget := path.Join(d.sharedPath(), file)
		releasePath := path.Join(releaseDir, file)

		script := fmt.Sprintf(
			"mkdir -p %[3]s && if [ -f %[2]s ] && [ ! -e %[1]s ]; then mv %[2]s %[1]s || true; fi && if [ ! -e %[1]s ]; then touch %[1]s; fi && rm -f %[2]s && mkdir -p %[4]s && ln -sfn %[1]s %[2]s",
			shell.Quote(sharedTarget),
			shell.Quote(releasePath),
			shell.Quote(path.Dir(sharedTarget)),
			shell.Quote(path.Dir(releasePath)),
		)

		if _, err := host.conn.Run(ctx, script); err != nil {
			return fmt.Errorf("cannot link shared file %s: %w", file, err)
		}
	}

	return nil
}

func (d *sshDeployer) runPreSwitchHooks(ctx context.Context, host deployHost, releaseDir string) error {
	if len(d.config.Hooks.PreSwitch) > 0 {
		return d.runHooks(ctx, host, d.config.Hooks.PreSwitch, releaseDir)
	}

	output, err := host.conn.Run(ctx, fmt.Sprintf("test -f %s && echo found || true", shell.Quote(path.Join(releaseDir, "vendor", "bin", "shopware-deployment-helper"))))
	if err != nil {
		return err
	}

	if !strings.Contains(output, "found") {
		logging.FromContext(ctx).Warnf("%sNo pre_switch hooks configured and vendor/bin/shopware-deployment-helper is missing, skipping application setup. Consider requiring shopware/deployment-helper in your project", d.logPrefix(host))
		return nil
	}

	return d.runHooks(ctx, host, []string{"vendor/bin/shopware-deployment-helper run"}, releaseDir)
}

func (d *sshDeployer) runHooks(ctx context.Context, host deployHost, hooks []string, dir string) error {
	for _, hook := range hooks {
		logging.FromContext(ctx).Infof("%sRunning remote hook: %s", d.logPrefix(host), hook)

		output, err := host.conn.Run(ctx, fmt.Sprintf("cd %s && { %s; }", shell.Quote(dir), hook))
		if err != nil {
			return fmt.Errorf("remote hook %q failed: %w", hook, err)
		}

		if trimmed := strings.TrimSpace(output); trimmed != "" {
			logging.FromContext(ctx).Infof("%s%s", d.logPrefix(host), trimmed)
		}
	}

	return nil
}

func (d *sshDeployer) switchCurrent(ctx context.Context, host deployHost, releaseDir string) error {
	tmpLink := d.currentPath() + ".tmp"

	// creating a new symlink and renaming it over the old one is atomic,
	// ln -sfn on its own is not
	_, err := host.conn.Run(ctx, fmt.Sprintf("ln -sfn %s %s && mv -fT %s %s", shell.Quote(releaseDir), shell.Quote(tmpLink), shell.Quote(tmpLink), shell.Quote(d.currentPath())))

	return err
}

func (d *sshDeployer) Releases(ctx context.Context) ([]HostReleases, error) {
	result := make([]HostReleases, len(d.hosts))

	group, groupCtx := errgroup.WithContext(ctx)

	for i, host := range d.hosts {
		group.Go(func() error {
			releases, err := d.hostReleases(groupCtx, host)
			if err != nil {
				return fmt.Errorf("host %s: %w", host.name, err)
			}

			result[i] = HostReleases{Host: host.name, Releases: releases}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, err
	}

	return result, nil
}

func (d *sshDeployer) hostReleases(ctx context.Context, host deployHost) ([]Release, error) {
	output, err := host.conn.Run(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null || true", shell.Quote(d.releasesPath())))
	if err != nil {
		return nil, err
	}

	activeOutput, err := host.conn.Run(ctx, fmt.Sprintf("readlink %s 2>/dev/null || true", shell.Quote(d.currentPath())))
	if err != nil {
		return nil, err
	}

	active := path.Base(strings.TrimSpace(activeOutput))

	badOutput, err := host.conn.Run(ctx, fmt.Sprintf("ls -1 %s/*/%s 2>/dev/null || true", shell.Quote(d.releasesPath()), badReleaseMarker))
	if err != nil {
		return nil, err
	}

	bad := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(badOutput), "\n") {
		if line == "" {
			continue
		}

		bad[path.Base(path.Dir(line))] = true
	}

	var releases []Release
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}

		releases = append(releases, Release{
			Name:   name,
			Active: name == active,
			Bad:    bad[name],
		})
	}

	slices.SortFunc(releases, func(a, b Release) int {
		return strings.Compare(a.Name, b.Name)
	})

	return releases, nil
}

func (d *sshDeployer) Rollback(ctx context.Context, target string) error {
	logger := logging.FromContext(ctx)

	hostReleases, err := d.Releases(ctx)
	if err != nil {
		return err
	}

	// the primary host decides which release to roll back to
	releases := hostReleases[0].Releases

	activeIdx := slices.IndexFunc(releases, func(r Release) bool { return r.Active })
	if activeIdx == -1 {
		return fmt.Errorf("cannot determine the active release, nothing to roll back")
	}

	active := releases[activeIdx]

	if target == "" {
		for i := activeIdx - 1; i >= 0; i-- {
			if releases[i].Bad {
				continue
			}

			target = releases[i].Name
			break
		}

		if target == "" {
			return fmt.Errorf("no earlier release available to roll back to")
		}
	} else {
		if target == active.Name {
			return fmt.Errorf("release %s is already active", target)
		}
	}

	// every host must have the target before any host is switched
	for _, hr := range hostReleases {
		if !slices.ContainsFunc(hr.Releases, func(r Release) bool { return r.Name == target }) {
			return fmt.Errorf("release %s does not exist on host %s", target, hr.Host)
		}
	}

	logger.Infof("Rolling back from release %s to %s", active.Name, target)

	for _, host := range d.hosts {
		if err := d.switchCurrent(ctx, host, path.Join(d.releasesPath(), target)); err != nil {
			return fmt.Errorf("host %s: %w", host.name, err)
		}

		// mark the release we rolled back from as bad so a later rollback skips it
		if _, err := host.conn.Run(ctx, fmt.Sprintf("touch %s", shell.Quote(path.Join(d.releasesPath(), active.Name, badReleaseMarker)))); err != nil {
			logger.Warnf("%sCannot mark release %s as bad: %v", d.logPrefix(host), active.Name, err)
		}
	}

	for _, host := range d.hosts {
		if err := d.runHooks(ctx, host, d.config.Hooks.PostSwitch, d.currentPath()); err != nil {
			return fmt.Errorf("host %s: %w", host.name, err)
		}
	}

	logger.Infof("Rolled back to release %s", target)

	return nil
}

func (d *sshDeployer) cleanupReleases(ctx context.Context, host deployHost) error {
	releases, err := d.hostReleases(ctx, host)
	if err != nil {
		return err
	}

	keep := d.keepReleases()
	if len(releases) <= keep {
		return nil
	}

	// releases are sorted ascending, the oldest come first
	obsolete := releases[:len(releases)-keep]

	for _, release := range obsolete {
		if release.Active {
			continue
		}

		logging.FromContext(ctx).Infof("%sRemoving old release %s", d.logPrefix(host), release.Name)

		if _, err := host.conn.Run(ctx, fmt.Sprintf("rm -rf %s", shell.Quote(path.Join(d.releasesPath(), release.Name)))); err != nil {
			return err
		}
	}

	return nil
}

func (d *sshDeployer) Close() error {
	var errs []error

	for _, host := range d.hosts {
		if err := host.conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}
