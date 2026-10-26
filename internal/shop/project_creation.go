package shop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

const (
	VersionLatest = "latest"
	// VersionTrunk is the Composer dev branch tracking Shopware's trunk on
	// GitHub. It installs like a released version, e.g.
	// `shopware-cli project create my-shop dev-trunk`.
	VersionTrunk = "dev-trunk"
)

// ValidateProjectFolder ensures that an existing target is an empty directory.
func ValidateProjectFolder(projectFolder string) error {
	info, err := os.Stat(projectFolder)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("project folder %s exists and is not a directory", projectFolder)
	}

	entries, err := os.ReadDir(projectFolder)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("the folder %s exists already and is not empty", projectFolder)
	}

	return nil
}

// ProjectFolder tracks directories owned by a project creation attempt. Its path
// is parsed once; cleanup is only enabled after the empty target is prepared.
type ProjectFolder struct {
	path     string
	created  []string
	prepared bool
}

// NewProjectFolder parses a non-empty path and rejects filesystem roots.
func NewProjectFolder(name string) (*ProjectFolder, error) {
	if name == "" {
		return nil, errors.New("cannot create project: project folder name must not be empty")
	}
	path, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve project folder: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if path == filepath.Dir(path) || (err == nil && resolved == filepath.Dir(resolved)) {
		return nil, errors.New("cannot create project in the filesystem root")
	}
	return &ProjectFolder{path: path}, nil
}

// Prepare validates the target immediately before writing and records only
// directories this attempt creates, including missing parents.
func (p *ProjectFolder) Prepare() error {
	if p == nil || p.path == "" {
		return errors.New("cannot prepare an uninitialized project folder")
	}
	if err := ValidateProjectFolder(p.path); err != nil {
		return err
	}
	var missing []string
	for dir := p.path; ; dir = filepath.Dir(dir) {
		_, err := os.Stat(dir)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		missing = append(missing, dir)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		if err := os.Mkdir(missing[i], os.ModePerm); err != nil {
			return err
		}
		p.created = append(p.created, missing[i])
	}
	p.prepared = true
	return nil
}

// Rollback preserves cause and joins any cleanup failure. Existing target
// directories are retained, and newly created parents are removed only if empty.
func (p *ProjectFolder) Rollback(ctx context.Context, cause error) error {
	if p == nil || (!p.prepared && len(p.created) == 0) {
		return cause
	}
	entries, err := os.ReadDir(p.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("cannot read partially created project in %s: %w", p.path, err))
	}
	if len(entries) == 0 && len(p.created) == 0 {
		return cause
	}
	logging.FromContext(ctx).Infof("Removing partially created project: %s", p.path)
	if p.prepared {
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(p.path, entry.Name())); err != nil {
				return errors.Join(cause, fmt.Errorf("cannot remove partially created project in %s: %w", p.path, err))
			}
		}
	}
	for i := len(p.created) - 1; i >= 0; i-- {
		if err := os.Remove(p.created[i]); err != nil && !errors.Is(err, os.ErrNotExist) {
			return errors.Join(cause, fmt.Errorf("cannot remove created directory %s: %w", p.created[i], err))
		}
	}
	p.created = nil
	p.prepared = false
	logging.FromContext(ctx).Infof("Removed partially created project: %s", p.path)
	return cause
}

// ValidateDeploymentMethod reports whether a deployment method is supported by
// the project scaffold.
func ValidateDeploymentMethod(deploymentMethod string) error {
	switch deploymentMethod {
	case DeploymentNone, DeploymentContainer, DeploymentDeployer, DeploymentPlatformSH, DeploymentShopwarePaaS:
		return nil
	default:
		return fmt.Errorf("invalid deployment method %q, allowed values: none, container, deployer, platformsh, shopware-paas", deploymentMethod)
	}
}

// ValidateCISystem reports whether a CI system is supported by the project
// scaffold.
func ValidateCISystem(ciSystem string) error {
	switch ciSystem {
	case CINone, CIGitHub, CIGitLab:
		return nil
	default:
		return fmt.Errorf("invalid CI system %q, allowed values: none, github, gitlab", ciSystem)
	}
}

// FilterInstallVersions returns supported Shopware releases in descending
// order, excluding dev branches and releases older than Shopware 6.4.18.0.
func FilterInstallVersions(releases []repository.Version) []*version.Version {
	filteredVersions := make([]*version.Version, 0)
	constraint, _ := version.NewConstraint(">=6.4.18.0")

	for _, release := range releases {
		if strings.HasPrefix(release.Version, "dev-") {
			continue
		}

		parsed, err := version.NewVersion(release.Version)
		if err != nil {
			continue
		}

		// Branch dev builds such as 6.7.12.x-dev parse successfully but are
		// not installable patch releases.
		if parsed.Prerelease() == "dev" {
			continue
		}

		if constraint.Check(parsed) {
			filteredVersions = append(filteredVersions, parsed)
		}
	}

	sort.Sort(sort.Reverse(version.Collection(filteredVersions)))
	for i, filteredVersion := range filteredVersions {
		filteredVersions[i], _ = version.NewVersion(strings.TrimPrefix(filteredVersion.String(), "v"))
	}

	return filteredVersions
}

// ResolveInstallVersion resolves "latest", exact releases, and dev branches
// to the version passed to Composer.
func ResolveInstallVersion(selectedVersion string, filteredVersions []*version.Version) (string, error) {
	if selectedVersion == VersionLatest {
		for _, filteredVersion := range filteredVersions {
			resolved := filteredVersion.String()
			if !strings.Contains(strings.ToLower(resolved), "rc") {
				return resolved, nil
			}
		}

		if len(filteredVersions) > 0 {
			return filteredVersions[0].String(), nil
		}
	}

	if strings.HasPrefix(selectedVersion, "dev-") {
		return selectedVersion, nil
	}

	for _, filteredVersion := range filteredVersions {
		if filteredVersion.String() == selectedVersion {
			return filteredVersion.String(), nil
		}
	}

	return "", fmt.Errorf("cannot find version %s", selectedVersion)
}
