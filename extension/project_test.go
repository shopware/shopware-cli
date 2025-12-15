package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetShopwareProjectConstraintComposerJson(t *testing.T) {
	testCases := []struct {
		Name       string
		Files      map[string]string
		Constraint string
		Error      string
	}{
		{
			Name: "Get constraint from composer.json",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "~6.5.0"
	}}`,
			},
			Constraint: "~6.5.0",
		},
		{
			Name: "Get constraint from composer.lock",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
				"composer.lock": `{
		"packages": [
{
"name": "shopware/core",
"version": "6.5.0"
}
]}`,
			},
			Constraint: "6.5.*",
		},
		{
			Name: "Branch installed, determine by Kernel.php",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
				"composer.lock": `{
		"packages": [
{
"name": "shopware/core",
"version": "dev-trunk"
}
]}`,
				"src/Core/composer.json": `{}`,
				"src/Core/Kernel.php": `<?php
final public const SHOPWARE_FALLBACK_VERSION = '6.6.9999999.9999999-dev';
`,
			},
			Constraint: "6.5.*",
		},
		{
			Name: "Get constraint from kernel (shopware/shopware case)",
			Files: map[string]string{
				"composer.json":          `{}`,
				"src/Core/composer.json": `{}`,
				"src/Core/Kernel.php": `<?php
final public const SHOPWARE_FALLBACK_VERSION = '6.6.9999999.9999999-dev';
`,
			},
			Constraint: "~6.6.0",
		},

		// error cases
		{
			Name:  "no composer.json",
			Files: map[string]string{},
			Error: "could not read composer.json",
		},

		{
			Name: "composer.json broken",
			Files: map[string]string{
				"composer.json": `broken`,
			},
			Error: "could not parse composer.json",
		},

		{
			Name: "composer.json with no shopware package",
			Files: map[string]string{
				"composer.json": `{}`,
			},
			Error: "missing shopware/core requirement in composer.json",
		},

		{
			Name: "composer.json malformed version, without lock, so we cannot fall down",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
			},
			Constraint: "6.5.*",
		},

		{
			Name: "composer.json malformed version, lock does not contain shopware/core",
			Files: map[string]string{
				"composer.json": `{
		"require": {
			"shopware/core": "6.5.*"
	}}`,
				"composer.lock": `{"packages": []}`,
			},
			Constraint: "6.5.*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			tmpDir := t.TempDir()

			for file, content := range tc.Files {
				tmpFile := filepath.Join(tmpDir, file)
				parentDir := filepath.Dir(tmpFile)

				if _, err := os.Stat(parentDir); os.IsNotExist(err) {
					assert.NoError(t, os.MkdirAll(parentDir, os.ModePerm))
				}

				assert.NoError(t, os.WriteFile(tmpFile, []byte(content), 0o644))
			}

			constraint, err := GetShopwareProjectConstraint(tmpDir)

			if tc.Constraint == "" {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), tc.Error)
				return
			}

			assert.NoError(t, err)

			assert.Equal(t, tc.Constraint, constraint.String())
		})
	}
}
