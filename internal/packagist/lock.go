package packagist

import (
	"encoding/json"
	"fmt"
	"os"
)

type ComposerLockPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ComposerLock struct {
	Packages []ComposerLockPackage `json:"packages"`
}

func (c *ComposerLock) GetPackage(name string) *ComposerLockPackage {
	for _, pkg := range c.Packages {
		if pkg.Name == name {
			return &pkg
		}
	}

	return nil
}

func ReadComposerLock(pathToFile string) (*ComposerLock, error) {
	content, err := os.ReadFile(pathToFile)
	if err != nil {
		return nil, err
	}

	var lock ComposerLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return nil, fmt.Errorf("could not parse composer.lock: %w", err)
	}

	return &lock, nil
}
