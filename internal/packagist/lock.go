package packagist

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/shyim/go-version"
)

type ComposerLockPackage struct {
	Name    string            `json:"name"`
	Version string            `json:"version"`
	Require map[string]string `json:"require"`
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

// ShopwareVersion returns the parsed version of the installed Shopware
// package (shopware/core, falling back to shopware/platform). Returns nil
// when no Shopware package is present or its version cannot be parsed.
// Composer lock entries are typically prefixed with "v" (e.g. "v6.6.10.0"),
// which is stripped before parsing.
func (c *ComposerLock) ShopwareVersion() *version.Version {
	for _, name := range []string{"shopware/core", "shopware/platform"} {
		pkg := c.GetPackage(name)
		if pkg == nil {
			continue
		}
		raw := strings.TrimPrefix(pkg.Version, "v")
		v, err := version.NewVersion(raw)
		if err != nil {
			continue
		}
		return v
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
