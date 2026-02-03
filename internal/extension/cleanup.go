package extension

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var (
	// These paths will only be removed relative to the top level of the plugin
	defaultNotAllowedPaths = []string{
		".editorconfig",
		".git",
		".github",
		".gitlab-ci.yml",
		".gitpod.Dockerfile",
		".gitpod.yml",
		".php-cs-fixer.cache",
		".php-cs-fixer.dist.php",
		".php_cs.cache",
		".php_cs.dist",
		".sw-zip-blacklist",
		".travis.yml",
		"ISSUE_TEMPLATE.md",
		"Makefile",
		"Resources/store",
		"auth.json",
		"bitbucket-pipelines.yml",
		"build.sh",
		"grumphp.yml",
		"phpstan.neon",
		"phpstan.neon.dist",
		"phpstan-baseline.neon",
		"phpunit.sh",
		"phpunit.xml.dist",
		"phpunitx.xml",
		"psalm.xml",
		"rector.php",
		"shell.nix",
		"src/Resources/app/administration/.tmp",
		"src/Resources/app/administration/node_modules",
		"src/Resources/app/node_modules",
		"src/Resources/app/storefront/node_modules",
		"src/Resources/store",
		"tests",
		"var",
	}

	// These files will be removed in all subdirectories
	defaultNotAllowedFiles = []string{
		".DS_Store",
		"Thumbs.db",
		"__MACOSX",
		".gitignore",
		".gitkeep",
		".prettierrc",
		"stylelint.config.js",
		".stylelintrc.js",
		".stylelintrc",
		"eslint.config.js",
		".eslintrc.js",
		".zipignore",
	}

	defaultNotAllowedExtensions = []string{
		".gz",
		".phar",
		".rar",
		".tar",
		".tar.gz",
		".zip",
	}
)

func CleanupExtensionFolder(path string, additionalPaths []string) error {
	notAllowedPaths := slices.Clone(defaultNotAllowedPaths)
	notAllowedPaths = append(notAllowedPaths, additionalPaths...)

	for _, folder := range notAllowedPaths {
		folderPath := filepath.Join(path, folder)
		if _, err := os.Stat(folderPath); !os.IsNotExist(err) {
			err := os.RemoveAll(folderPath)
			if err != nil {
				return err
			}
		}
	}

	return filepath.Walk(path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// When we delete a folder, this function will be called also the files in it
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil
		}

		base := filepath.Base(path)

		for _, file := range defaultNotAllowedFiles {
			if file == base {
				return os.RemoveAll(path)
			}
		}

		for _, ext := range defaultNotAllowedExtensions {
			if strings.HasSuffix(base, ext) {
				return os.RemoveAll(path)
			}
		}

		return nil
	})
}
