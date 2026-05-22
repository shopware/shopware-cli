package packagist

import (
	"encoding/json"
	"fmt"
	"os"
)

type ComposerLockPackageDist struct {
	Type      string `json:"type,omitempty"`
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
	Shasum    string `json:"shasum,omitempty"`
}

type ComposerLockPackageSource struct {
	Type      string `json:"type,omitempty"`
	URL       string `json:"url,omitempty"`
	Reference string `json:"reference,omitempty"`
}

type ComposerLockPackage struct {
	Name        string                    `json:"name"`
	Version     string                    `json:"version"`
	Type        string                    `json:"type,omitempty"`
	Require     map[string]string         `json:"require"`
	License     []string                  `json:"license,omitempty"`
	Description string                    `json:"description,omitempty"`
	Homepage    string                    `json:"homepage,omitempty"`
	Time        string                    `json:"time,omitempty"`
	Dist        ComposerLockPackageDist   `json:"dist,omitempty"`
	Source      ComposerLockPackageSource `json:"source,omitempty"`
}

type ComposerLock struct {
	Packages    []ComposerLockPackage `json:"packages"`
	PackagesDev []ComposerLockPackage `json:"packages-dev"`
}

func (c *ComposerLock) GetPackage(name string) *ComposerLockPackage {
	for _, pkg := range c.Packages {
		if pkg.Name == name {
			return &pkg
		}
	}

	return nil
}

// ShopwarePHPConstraint returns the `require.php` constraint declared by the
// project's installed Shopware package (shopware/core, falling back to
// shopware/platform). Returns nil when no Shopware package is present or it
// declares no PHP requirement.
func (c *ComposerLock) ShopwarePHPConstraint() *PHPConstraint {
	for _, name := range []string{"shopware/core", "shopware/platform"} {
		pkg := c.GetPackage(name)
		if pkg == nil {
			continue
		}
		if php, ok := pkg.Require["php"]; ok && php != "" {
			return NewPHPConstraint(php)
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
