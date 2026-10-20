package shop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
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

// ProjectFolderExists reports whether the project folder already exists.
func ProjectFolderExists(projectFolder string) (bool, error) {
	if projectFolder == "" {
		return false, errors.New("project folder must not be empty")
	}
	if _, err := os.Stat(projectFolder); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CleanupFailedProjectCreation removes what a failed creation wrote into the
// project folder. A folder that existed before is kept and only emptied.
func CleanupFailedProjectCreation(projectFolder string, existedBefore bool) error {
	if projectFolder == "" {
		return errors.New("cannot remove partially created project: project folder must not be empty")
	}

	absFolder, err := filepath.Abs(projectFolder)
	if err != nil {
		return fmt.Errorf("cannot remove partially created project in %s: %w", projectFolder, err)
	}
	if absFolder == filepath.Dir(absFolder) {
		return fmt.Errorf("cannot remove partially created project in %s: it is the filesystem root", projectFolder)
	}
	if resolved, err := filepath.EvalSymlinks(absFolder); err == nil && resolved == filepath.Dir(resolved) {
		return fmt.Errorf("cannot remove partially created project in %s: it is the filesystem root", projectFolder)
	}

	if !existedBefore {
		if err := os.RemoveAll(absFolder); err != nil {
			return fmt.Errorf("cannot remove partially created project in %s: %w", projectFolder, err)
		}
		return nil
	}

	entries, err := os.ReadDir(absFolder)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot remove partially created project in %s: %w", projectFolder, err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(absFolder, entry.Name())); err != nil {
			return fmt.Errorf("cannot remove partially created project in %s: %w", projectFolder, err)
		}
	}
	return nil
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
