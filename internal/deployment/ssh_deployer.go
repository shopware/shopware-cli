package deployment

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

const badReleaseMarker = "BAD_RELEASE"

var (
	defaultSharedFiles = []string{".env", "install.lock"}
	defaultSharedDirs  = []string{"files", "public/media", "public/thumbnail", "public/sitemap", "var/log"}
)

const defaultKeepReleases = 5

// sshDeployer deploys a project using a releases/shared directory layout and an
// atomically switched "current" symlink, similar to what Deployer (deployer.org) does.
type sshDeployer struct {
	projectRoot string
	deployPath  string
	config      *shop.EnvironmentDeployment
	conn        Connection

	// injected for tests
	now      func() time.Time
	runLocal func(ctx context.Context, dir string, command string) error
}

func newSSHDeployer(projectRoot string, env *shop.EnvironmentConfig, _ *shop.Config) (Deployer, error) {
	if env.Deployment == nil || env.Deployment.Path == "" {
		return nil, fmt.Errorf("the environment is missing the deployment.path setting")
	}

	conn, err := newSSHConnection(env.SSH)
	if err != nil {
		return nil, err
	}

	return &sshDeployer{
		projectRoot: projectRoot,
		deployPath:  strings.TrimRight(env.Deployment.Path, "/"),
		config:      env.Deployment,
		conn:        conn,
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

	logger.Infof("Preparing deployment structure in %s", d.deployPath)

	if _, err := d.conn.Run(ctx, fmt.Sprintf("mkdir -p %s %s", shQuote(d.releasesPath()), shQuote(d.sharedPath()))); err != nil {
		return err
	}

	if _, err := d.conn.Run(ctx, fmt.Sprintf("[ ! -e %[1]s ] || [ -L %[1]s ]", shQuote(d.currentPath()))); err != nil {
		return fmt.Errorf("%s exists but is not a symlink, refusing to deploy into an unmanaged directory", d.currentPath())
	}

	if _, err := d.conn.Run(ctx, fmt.Sprintf("mkdir -p %s", shQuote(releaseDir))); err != nil {
		return err
	}

	logger.Infof("Uploading project to release %s", releaseName)

	if err := d.upload(ctx, releaseDir); err != nil {
		return err
	}

	logger.Infof("Linking shared files and directories")

	if err := d.linkShared(ctx, releaseDir); err != nil {
		return err
	}

	if err := d.runPreSwitchHooks(ctx, releaseDir); err != nil {
		return err
	}

	logger.Infof("Switching %s to release %s", d.currentPath(), releaseName)

	if err := d.switchCurrent(ctx, releaseDir); err != nil {
		return err
	}

	if err := d.runHooks(ctx, d.config.Hooks.PostSwitch, d.currentPath()); err != nil {
		return err
	}

	if err := d.cleanupReleases(ctx); err != nil {
		logger.Warnf("Cleanup of old releases failed: %v", err)
	}

	logger.Infof("Deployed release %s", releaseName)

	return nil
}

func (d *sshDeployer) upload(ctx context.Context, releaseDir string) error {
	exclude := slices.Concat(d.config.Exclude, d.sharedDirs(), d.sharedFiles())

	reader, writer := io.Pipe()

	go func() {
		writer.CloseWithError(writeProjectArchive(writer, d.projectRoot, exclude))
	}()

	if err := d.conn.Stream(ctx, fmt.Sprintf("tar -xzpf - -C %s", shQuote(releaseDir)), reader); err != nil {
		_ = reader.CloseWithError(err)
		return fmt.Errorf("upload failed: %w", err)
	}

	// var/cache and var/log are excluded from the upload but Shopware expects them
	_, err := d.conn.Run(ctx, fmt.Sprintf("mkdir -p %s %s", shQuote(path.Join(releaseDir, "var", "cache")), shQuote(path.Join(releaseDir, "var", "log"))))

	return err
}

func (d *sshDeployer) linkShared(ctx context.Context, releaseDir string) error {
	for _, dir := range d.sharedDirs() {
		dir = strings.Trim(dir, "/")
		sharedTarget := path.Join(d.sharedPath(), dir)
		releasePath := path.Join(releaseDir, dir)

		// seed the shared directory from the release on first deployment, afterwards
		// drop the uploaded variant and replace it with a symlink into shared/
		script := fmt.Sprintf(
			"if [ -d %[2]s ] && [ ! -e %[1]s ]; then mkdir -p %[3]s && mv %[2]s %[1]s; fi && mkdir -p %[1]s && rm -rf %[2]s && mkdir -p %[4]s && ln -s %[1]s %[2]s",
			shQuote(sharedTarget),
			shQuote(releasePath),
			shQuote(path.Dir(sharedTarget)),
			shQuote(path.Dir(releasePath)),
		)

		if _, err := d.conn.Run(ctx, script); err != nil {
			return fmt.Errorf("cannot link shared directory %s: %w", dir, err)
		}
	}

	for _, file := range d.sharedFiles() {
		file = strings.Trim(file, "/")
		sharedTarget := path.Join(d.sharedPath(), file)
		releasePath := path.Join(releaseDir, file)

		script := fmt.Sprintf(
			"mkdir -p %[3]s && if [ -f %[2]s ] && [ ! -e %[1]s ]; then mv %[2]s %[1]s; fi && if [ ! -e %[1]s ]; then touch %[1]s; fi && rm -f %[2]s && mkdir -p %[4]s && ln -s %[1]s %[2]s",
			shQuote(sharedTarget),
			shQuote(releasePath),
			shQuote(path.Dir(sharedTarget)),
			shQuote(path.Dir(releasePath)),
		)

		if _, err := d.conn.Run(ctx, script); err != nil {
			return fmt.Errorf("cannot link shared file %s: %w", file, err)
		}
	}

	return nil
}

func (d *sshDeployer) runPreSwitchHooks(ctx context.Context, releaseDir string) error {
	if len(d.config.Hooks.PreSwitch) > 0 {
		return d.runHooks(ctx, d.config.Hooks.PreSwitch, releaseDir)
	}

	output, err := d.conn.Run(ctx, fmt.Sprintf("test -f %s && echo found || true", shQuote(path.Join(releaseDir, "vendor", "bin", "shopware-deployment-helper"))))
	if err != nil {
		return err
	}

	if !strings.Contains(output, "found") {
		logging.FromContext(ctx).Warnf("No pre_switch hooks configured and vendor/bin/shopware-deployment-helper is missing, skipping application setup. Consider requiring shopware/deployment-helper in your project")
		return nil
	}

	return d.runHooks(ctx, []string{"vendor/bin/shopware-deployment-helper run"}, releaseDir)
}

func (d *sshDeployer) runHooks(ctx context.Context, hooks []string, dir string) error {
	for _, hook := range hooks {
		logging.FromContext(ctx).Infof("Running remote hook: %s", hook)

		output, err := d.conn.Run(ctx, fmt.Sprintf("cd %s && { %s; }", shQuote(dir), hook))
		if err != nil {
			return fmt.Errorf("remote hook %q failed: %w", hook, err)
		}

		if trimmed := strings.TrimSpace(output); trimmed != "" {
			logging.FromContext(ctx).Infof("%s", trimmed)
		}
	}

	return nil
}

func (d *sshDeployer) switchCurrent(ctx context.Context, releaseDir string) error {
	tmpLink := d.currentPath() + ".tmp"

	// creating a new symlink and renaming it over the old one is atomic,
	// ln -sfn on its own is not
	_, err := d.conn.Run(ctx, fmt.Sprintf("ln -sfn %s %s && mv -fT %s %s", shQuote(releaseDir), shQuote(tmpLink), shQuote(tmpLink), shQuote(d.currentPath())))

	return err
}

func (d *sshDeployer) Releases(ctx context.Context) ([]Release, error) {
	output, err := d.conn.Run(ctx, fmt.Sprintf("ls -1 %s 2>/dev/null || true", shQuote(d.releasesPath())))
	if err != nil {
		return nil, err
	}

	activeOutput, err := d.conn.Run(ctx, fmt.Sprintf("readlink %s 2>/dev/null || true", shQuote(d.currentPath())))
	if err != nil {
		return nil, err
	}

	active := path.Base(strings.TrimSpace(activeOutput))

	badOutput, err := d.conn.Run(ctx, fmt.Sprintf("ls -1 %s/*/%s 2>/dev/null || true", shQuote(d.releasesPath()), badReleaseMarker))
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

	releases, err := d.Releases(ctx)
	if err != nil {
		return err
	}

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

		if !slices.ContainsFunc(releases, func(r Release) bool { return r.Name == target }) {
			return fmt.Errorf("release %s does not exist on the server", target)
		}
	}

	logger.Infof("Rolling back from release %s to %s", active.Name, target)

	if err := d.switchCurrent(ctx, path.Join(d.releasesPath(), target)); err != nil {
		return err
	}

	// mark the release we rolled back from as bad so a later rollback skips it
	if _, err := d.conn.Run(ctx, fmt.Sprintf("touch %s", shQuote(path.Join(d.releasesPath(), active.Name, badReleaseMarker)))); err != nil {
		logger.Warnf("Cannot mark release %s as bad: %v", active.Name, err)
	}

	if err := d.runHooks(ctx, d.config.Hooks.PostSwitch, d.currentPath()); err != nil {
		return err
	}

	logger.Infof("Rolled back to release %s", target)

	return nil
}

func (d *sshDeployer) cleanupReleases(ctx context.Context) error {
	releases, err := d.Releases(ctx)
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

		logging.FromContext(ctx).Infof("Removing old release %s", release.Name)

		if _, err := d.conn.Run(ctx, fmt.Sprintf("rm -rf %s", shQuote(path.Join(d.releasesPath(), release.Name)))); err != nil {
			return err
		}
	}

	return nil
}

func (d *sshDeployer) Close() error {
	return d.conn.Close()
}
