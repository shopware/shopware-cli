// Package deploymenthelper manages the deployment helper's Composer requirement.
package deploymenthelper

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/shyim/go-composer"
)

const Package = "shopware/deployment-helper"

// ErrNotRequired indicates that production installs will omit the helper.
var ErrNotRequired = errors.New("cannot use deployment helper without shopware/deployment-helper in composer.json require: run \"composer require shopware/deployment-helper\"")

// CheckRequired verifies that the project requires the helper in production.
func CheckRequired(projectRoot string) error {
	c, err := composer.ReadJson(filepath.Join(projectRoot, "composer.json"))
	if err != nil {
		return fmt.Errorf("cannot read project composer.json: %w", err)
	}
	if !c.HasPackage(Package) {
		return ErrNotRequired
	}
	return nil
}

// EnsureRequired adds the helper to require, preserving an existing constraint
// when moving it from require-dev. It reports whether the manifest changed.
func EnsureRequired(c *composer.Json) bool {
	changed := false
	if !c.HasPackage(Package) {
		constraint := "*"
		if c.HasPackageDev(Package) {
			constraint = c.RequireDev[Package]
		}
		c.AddPackage(Package, constraint)
		changed = true
	}
	if c.HasPackageDev(Package) {
		c.RemovePackageDev(Package)
		changed = true
	}
	return changed
}
