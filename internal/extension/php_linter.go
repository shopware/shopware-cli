package extension

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/shyim/go-phplint"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

func validatePHPFiles(c context.Context, ext Extension, check validation.Check, phpVersion string) {
	if ext.GetType() != TypePlatformPlugin {
		return
	}

	constraint, err := ext.GetShopwareVersionConstraint()
	if err != nil {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "php.linter",
			Message:    "Could not parse shopware version constraint: " + err.Error(),
			Severity:   validation.SeverityError,
		})
		return
	}

	if cfg := ext.GetExtensionConfig(); cfg != nil && cfg.Validation.PhpVersion != "" {
		phpVersion = cfg.Validation.PhpVersion
	}

	if phpVersion != "" {
		phpVersion = normalizePhpVersion(phpVersion)
	} else {
		phpVersion, err = GetPhpVersion(c, constraint)
		if err != nil {
			check.AddResult(validation.CheckResult{
				Path:       "composer.json",
				Identifier: "php.linter",
				Message:    "Could not find min php version for plugin: " + err.Error(),
				Severity:   validation.SeverityWarning,
			})
			return
		}
	}

	if phpVersion == "7.2" {
		phpVersion = "7.3"
		logging.FromContext(c).Infof("PHP 7.2 is not supported for PHP linting, using 7.3 now")
	}

	ver, err := phplint.ParseVersion(phpVersion)
	if err != nil {
		check.AddResult(validation.CheckResult{
			Path:       "composer.json",
			Identifier: "php.linter",
			Message:    "Could not parse php version: " + err.Error(),
			Severity:   validation.SeverityWarning,
		})
		return
	}

	for _, val := range ext.GetSourceDirs() {
		_ = filepath.Walk(val, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() || !strings.HasSuffix(path, ".php") {
				return nil
			}

			relPath := validation.NormalizeSourcePath(path, ext.GetPath())

			content, err := os.ReadFile(path)
			if err != nil {
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Identifier: "php.linter",
					Message:    "Could not read php file: " + err.Error(),
					Severity:   validation.SeverityWarning,
				})
				// The unreadable file is reported as a warning; the walk goes on.
				return nil //nolint:nilerr
			}

			diags, err := phplint.Lint(relPath, content, phplint.Options{PHPVersion: ver})
			if err != nil {
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Identifier: "php.linter",
					Message:    "Could not lint php file: " + err.Error(),
					Severity:   validation.SeverityWarning,
				})
				// The unlintable file is reported as a warning; the walk goes on.
				return nil //nolint:nilerr
			}

			for _, diag := range diags {
				check.AddResult(validation.CheckResult{
					Path:       relPath,
					Identifier: "php.linter",
					Message:    diag.Message,
					Line:       diag.Start.Line,
					Severity:   validation.SeverityError,
				})
			}

			return nil
		})
	}
}

// normalizePhpVersion trims a PHP version string to the major.minor form expected by the phplint package.
func normalizePhpVersion(v string) string {
	v = strings.TrimSpace(v)
	parts := strings.Split(v, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return v
}

// phpVersionURL can be overridden in tests to use a mock server
var phpVersionURL = "https://raw.githubusercontent.com/FriendsOfShopware/shopware-static-data/main/data/php-version.json"

func GetPhpVersion(ctx context.Context, constraint *version.Constraints) (string, error) {
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, phpVersionURL, http.NoBody)

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return "", err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("GetPhpVersion: %v", err)
		}
	}()

	var shopwareToPHPVersion map[string]string

	err = json.NewDecoder(resp.Body).Decode(&shopwareToPHPVersion)
	if err != nil {
		return "", err
	}

	// Map iteration order is random; check the Shopware versions in ascending
	// order so the lowest supported Shopware version decides the PHP version.
	shopwareVersions := make([]*version.Version, 0, len(shopwareToPHPVersion))
	for shopwareVersion := range shopwareToPHPVersion {
		v, err := version.NewVersion(shopwareVersion)
		if err != nil {
			continue
		}
		shopwareVersions = append(shopwareVersions, v)
	}
	slices.SortFunc(shopwareVersions, func(a, b *version.Version) int {
		return a.Compare(b)
	})

	for _, shopwareVersion := range shopwareVersions {
		if constraint.Check(shopwareVersion) {
			return shopwareToPHPVersion[shopwareVersion.Original()], nil
		}
	}

	return "", errors.New("could not find php version for shopware version")
}
