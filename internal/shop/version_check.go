package shop

import (
	"encoding/json"
	"errors"
	"os"
	"path"

	"github.com/shyim/go-version"
)

var (
	ErrNoComposerFileFound        = errors.New("could not determine Shopware version as no composer.json or composer.lock file was found")
	ErrShopwareDependencyNotFound = errors.New("could not determine Shopware version as no shopware/core dependency was found")
)

func IsShopwareVersion(projectRoot string, requiredVersion string) (bool, error) {
	composerJson := path.Join(projectRoot, "composer.json")
	composerLock := path.Join(projectRoot, "composer.lock")

	if _, err := os.Stat(composerLock); err == nil {
		found, err := determineByComposerLock(composerLock, requiredVersion)

		if !errors.Is(err, ErrShopwareDependencyNotFound) {
			return found, err
		}
	}

	if _, err := os.Stat(composerJson); err == nil {
		return determineByComposerJson(composerJson)
	}

	return false, ErrNoComposerFileFound
}

type composerLockStruct struct {
	Packages []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"packages"`
}

func determineByComposerLock(composerLock, requiredVersion string) (bool, error) {
	bytes, err := os.ReadFile(composerLock)
	if err != nil {
		return false, err
	}

	var lock composerLockStruct
	if err := json.Unmarshal(bytes, &lock); err != nil {
		return false, err
	}

	constraint := version.MustConstraints(version.NewConstraint(requiredVersion))

	for _, pkg := range lock.Packages {
		if pkg.Name == "shopware/core" {
			if constraint.Check(version.Must(version.NewVersion(pkg.Version))) {
				return true, nil
			}

			return false, nil
		}
	}

	return false, ErrShopwareDependencyNotFound
}

type composerJsonStruct struct {
	Name string `json:"name"`
}

func determineByComposerJson(composerJson string) (bool, error) {
	bytes, err := os.ReadFile(composerJson)
	if err != nil {
		return false, err
	}

	var jsonStruct composerJsonStruct
	if err := json.Unmarshal(bytes, &jsonStruct); err != nil {
		return false, err
	}

	if jsonStruct.Name == "shopware/platform" {
		return true, nil
	}

	return false, ErrShopwareDependencyNotFound
}
