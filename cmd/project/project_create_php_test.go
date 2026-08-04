package project

import (
	"context"
	"fmt"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// stubDiscovery replaces the PHP discovery for the duration of the test so
// tests never depend on the PHP versions installed on the machine.
func stubDiscovery(t *testing.T, installations []system.PHPInstallation) {
	t.Helper()
	original := discoverPHPInstallations
	discoverPHPInstallations = func(context.Context) []system.PHPInstallation {
		return installations
	}
	t.Cleanup(func() { discoverPHPInstallations = original })
}

func TestResolveLocalPHPExplicitVersion(t *testing.T) {
	stubDiscovery(t, []system.PHPInstallation{
		{Binary: "/php84", Version: "8.4.2", Source: system.PHPSourcePath, Default: true},
		{Binary: "/php83-new", Version: "8.3.33", Source: "homebrew"},
		{Binary: "/php83-old", Version: "8.3.7", Source: "homebrew"},
	})

	explicit := func(phpVersion string) createOptions {
		return createOptions{phpVersion: phpVersion, phpVersionExplicit: true}
	}

	t.Run("resolves the newest patch release of the requested version", func(t *testing.T) {
		opts := explicit("8.3")
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint("~8.3.0"))
		assert.NoError(t, err)
		assert.Equal(t, "/php83-new", opts.phpBinary)
		assert.Equal(t, "8.3", opts.phpVersion)
	})

	t.Run("the requested version wins over the newer PATH default", func(t *testing.T) {
		opts := explicit("8.3")
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint(">=8.2"))
		assert.NoError(t, err)
		assert.Equal(t, "/php83-new", opts.phpBinary)
	})

	t.Run("a version not installed is rejected", func(t *testing.T) {
		opts := explicit("8.1")
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint(">=8.0"))

		var notFound *system.PHPVersionNotFoundError
		assert.ErrorAs(t, err, &notFound)
		assert.Equal(t, "8.1", notFound.Pin)
	})

	t.Run("a version the Shopware release does not support is rejected", func(t *testing.T) {
		opts := explicit("8.3")
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint("~8.4.0"))
		assert.ErrorContains(t, err, "8.3")
		assert.ErrorContains(t, err, "~8.4.0")
		assert.ErrorContains(t, err, "--php-version")
	})
}

// Replacing the form's pick would silently contradict the summary the user
// confirmed.
func TestResolveLocalPHPKeepsInteractiveSelection(t *testing.T) {
	stubDiscovery(t, []system.PHPInstallation{
		{Binary: "/usr/bin/php", Version: "8.4.2", Source: system.PHPSourcePath, Default: true},
		{Binary: "/opt/php83", Version: "8.3.7", Source: "homebrew"},
	})

	t.Run("a selection made in the form survives", func(t *testing.T) {
		opts := createOptions{interactive: true, phpBinary: "/opt/php83", phpVersion: "8.3"}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint(">=8.2"))
		assert.NoError(t, err)
		assert.Equal(t, "/opt/php83", opts.phpBinary)
		assert.Equal(t, "8.3", opts.phpVersion)
	})

	t.Run("an empty selection is still filled in", func(t *testing.T) {
		opts := createOptions{interactive: true}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint(">=8.2"))
		assert.NoError(t, err)
		assert.Equal(t, "/usr/bin/php", opts.phpBinary)
	})
}

func TestResolveLocalPHPRejectsUnusablePHPBinaryEnv(t *testing.T) {
	stubDiscovery(t, []system.PHPInstallation{
		{Binary: "/usr/bin/php", Version: "8.4.2", Source: system.PHPSourcePath, Default: true},
	})

	// Stub what the real helper returns, wrapping included.
	original := unusablePHPBinaryEnv
	unusablePHPBinaryEnv = func(context.Context) error {
		return fmt.Errorf("PHP_BINARY is set but unusable: %w", &system.PHPBinaryError{Path: "php8.2", Reason: "is not an executable file"})
	}
	t.Cleanup(func() { unusablePHPBinaryEnv = original })

	opts := createOptions{}
	err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint(">=8.2"))
	assert.ErrorContains(t, err, "PHP_BINARY is set but unusable")
	assert.ErrorContains(t, err, "php8.2")
	assert.Empty(t, opts.phpBinary)
}

func TestResolveLocalPHPNonInteractiveFallback(t *testing.T) {
	t.Run("prefers compatible PHP_BINARY over newer PATH candidate", func(t *testing.T) {
		stubDiscovery(t, []system.PHPInstallation{
			{Binary: "/env/php", Version: "8.3.1", Source: system.PHPSourceEnv},
			{Binary: "/usr/bin/php", Version: "8.4.2", Source: system.PHPSourcePath, Default: true},
		})

		opts := createOptions{}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint(">=8.2"))
		assert.NoError(t, err)
		assert.Equal(t, "/env/php", opts.phpBinary)
	})

	t.Run("falls back to PATH when PHP_BINARY is incompatible", func(t *testing.T) {
		stubDiscovery(t, []system.PHPInstallation{
			{Binary: "/env/php", Version: "8.1.0", Source: system.PHPSourceEnv},
			{Binary: "/usr/bin/php", Version: "8.3.2", Source: system.PHPSourcePath, Default: true},
		})

		opts := createOptions{}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint("~8.3.0"))
		assert.NoError(t, err)
		assert.Equal(t, "/usr/bin/php", opts.phpBinary)
	})

	t.Run("fails with flag hint when only other sources are compatible", func(t *testing.T) {
		stubDiscovery(t, []system.PHPInstallation{
			{Binary: "/usr/bin/php", Version: "8.1.0", Source: system.PHPSourcePath, Default: true},
			{Binary: "/opt/homebrew/opt/php@8.3/bin/php", Version: "8.3.2", Source: "homebrew"},
		})

		opts := createOptions{}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint("~8.3.0"))
		assert.ErrorContains(t, err, "--php-version")
		assert.ErrorContains(t, err, "/opt/homebrew/opt/php@8.3/bin/php")
		assert.Empty(t, opts.phpBinary)
	})

	t.Run("leaves selection empty when nothing is compatible", func(t *testing.T) {
		stubDiscovery(t, []system.PHPInstallation{
			{Binary: "/usr/bin/php", Version: "8.1.0", Source: system.PHPSourcePath, Default: true},
		})

		opts := createOptions{}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint("~8.3.0"))
		assert.NoError(t, err)
		assert.Empty(t, opts.phpBinary)
	})

	t.Run("leaves selection empty when nothing is discovered", func(t *testing.T) {
		stubDiscovery(t, nil)

		opts := createOptions{}
		err := resolveLocalPHP(t.Context(), &opts, shop.NewPHPConstraint("~8.3.0"))
		assert.NoError(t, err)
		assert.Empty(t, opts.phpBinary)
	})
}

func TestCompatiblePHPFor(t *testing.T) {
	releases := []repository.Version{
		{Version: "v6.6.0.0", Require: map[string]string{"php": "~8.2.0 || ~8.3.0"}},
		{Version: "v6.7.0.0", Require: map[string]string{"php": "~8.3.0 || ~8.4.0"}},
	}
	filteredVersions := []*version.Version{
		version.Must(version.NewVersion("6.7.0.0")),
		version.Must(version.NewVersion("6.6.0.0")),
	}

	stubDiscovery(t, []system.PHPInstallation{
		{Binary: "/php84", Version: "8.4.1", Source: system.PHPSourcePath, Default: true},
		{Binary: "/php83", Version: "8.3.2", Source: "homebrew"},
		{Binary: "/php82", Version: "8.2.9", Source: "homebrew"},
	})

	binaries := func(installations []system.PHPInstallation) []string {
		out := make([]string, 0, len(installations))
		for _, installation := range installations {
			out = append(out, installation.Binary)
		}
		return out
	}

	t.Run("filters by the constraint of the selected version", func(t *testing.T) {
		got := compatiblePHPFor(t.Context(), releases, "6.6.0.0", filteredVersions)
		assert.Equal(t, []string{"/php83", "/php82"}, binaries(got))
	})

	t.Run("refilters when another version is selected", func(t *testing.T) {
		got := compatiblePHPFor(t.Context(), releases, "6.7.0.0", filteredVersions)
		assert.Equal(t, []string{"/php84", "/php83"}, binaries(got))
	})

	t.Run("resolves latest to a concrete version", func(t *testing.T) {
		got := compatiblePHPFor(t.Context(), releases, shop.VersionLatest, filteredVersions)
		assert.Equal(t, []string{"/php84", "/php83"}, binaries(got))
	})

	t.Run("returns nothing for an unknown version", func(t *testing.T) {
		assert.Empty(t, compatiblePHPFor(t.Context(), releases, "6.1.0.0", filteredVersions))
	})
}

func TestShouldPromptPHPSelection(t *testing.T) {
	assert.True(t, shouldPromptPHPSelection(2))
	assert.False(t, shouldPromptPHPSelection(1))
	assert.False(t, shouldPromptPHPSelection(0))
}

func TestPHPVersionOptions(t *testing.T) {
	options := phpVersionOptions([]string{"8.2", "8.3"})

	assert.Len(t, options, 2)
	assert.Equal(t, "PHP 8.2", options[0].Key)
	assert.Equal(t, "8.2", options[0].Value)
}

func TestHighestOrEmpty(t *testing.T) {
	// SupportedPHPVersions is ordered lowest to highest.
	assert.Equal(t, "8.5", highestOrEmpty([]string{"8.3", "8.4", "8.5"}))
	assert.Empty(t, highestOrEmpty(nil))
}

func TestPHPConstraintForDockerImages(t *testing.T) {
	releases := []repository.Version{
		{Version: "v6.6.0.0", Require: map[string]string{"php": "~8.2.0 || ~8.3.0"}},
		{Version: "v6.7.0.0", Require: map[string]string{"php": "~8.3.0 || ~8.4.0"}},
	}
	filteredVersions := []*version.Version{
		version.Must(version.NewVersion("6.7.0.0")),
		version.Must(version.NewVersion("6.6.0.0")),
	}

	// Docker offers image tags from SupportedPHPVersions filtered by the release,
	// with no dependency on what is installed locally.
	assert.Equal(t, []string{"8.2", "8.3"},
		phpConstraintFor(releases, "6.6.0.0", filteredVersions).SupportedVersions())
	assert.Equal(t, []string{"8.3", "8.4"},
		phpConstraintFor(releases, "6.7.0.0", filteredVersions).SupportedVersions())
}

// huh dispatches OptionsFunc asynchronously but evaluates hide funcs during
// navigation, so deciding visibility from its options hides the group forever.
func TestPHPSelectionVisibilityDoesNotDependOnOptionsFunc(t *testing.T) {
	releases := []repository.Version{
		{Version: "v6.6.0.0", Require: map[string]string{"php": ">=8.2"}},
	}
	filteredVersions := []*version.Version{version.Must(version.NewVersion("6.6.0.0"))}

	stubDiscovery(t, []system.PHPInstallation{
		{Binary: "/php83", Version: "8.3.2", Source: system.PHPSourcePath, Default: true},
		{Binary: "/php82", Version: "8.2.9", Source: "homebrew"},
	})

	// Mirrors what the form does before any field is initialized.
	compatible := compatiblePHPFor(t.Context(), releases, "6.6.0.0", filteredVersions)
	assert.True(t, shouldPromptPHPSelection(len(compatible)))
}

func TestPHPInstallationOptions(t *testing.T) {
	options := phpInstallationOptions([]system.PHPInstallation{
		{Binary: "/php83", Version: "8.3.2", Source: "homebrew"},
		{Binary: "/php82", Version: "8.2.9"},
	})

	assert.Len(t, options, 2)
	assert.Contains(t, options[0].Key, "homebrew")
	assert.Equal(t, "/php83", options[0].Value)
	// A blank source must not render an empty "()" suffix.
	assert.NotContains(t, options[1].Key, "(")
	assert.Equal(t, "/php82", options[1].Value)
}

func TestSetPHPRecordsPortableVersion(t *testing.T) {
	opts := createOptions{}
	opts.setPHP(system.PHPInstallation{Binary: "/opt/homebrew/Cellar/php@8.3/8.3.33/bin/php", Version: "8.3.33"})

	// Only the major.minor version reaches the config.
	assert.Equal(t, "/opt/homebrew/Cellar/php@8.3/8.3.33/bin/php", opts.phpBinary)
	assert.Equal(t, "8.3", opts.phpVersion)

	opts.clearPHP()
	assert.Empty(t, opts.phpBinary)
	assert.Empty(t, opts.phpVersion)
}

// An explicit --php-version still applies to Docker: it selects the image tag.
func TestClearPHPKeepsExplicitlyRequestedVersion(t *testing.T) {
	opts := createOptions{phpVersion: "8.3", phpVersionExplicit: true, phpBinary: "/php83"}

	opts.clearPHP()
	assert.Empty(t, opts.phpBinary)
	assert.Equal(t, "8.3", opts.phpVersion)
}

func TestValidateAndPreflightDockerAcceptsRequestedPHPVersion(t *testing.T) {
	releases := []repository.Version{
		{Version: "v6.7.0.0", Require: map[string]string{"php": "~8.3.0 || ~8.4.0"}},
	}
	filteredVersions := []*version.Version{version.Must(version.NewVersion("6.7.0.0"))}

	// A supported version is not asserted here: validateAndPreflight continues into
	// the security-advisory check, which queries Packagist over the network.

	t.Run("a version the Shopware release does not support is rejected", func(t *testing.T) {
		stubDiscovery(t, nil)
		opts := createOptions{
			projectFolder: "my-shop",
			useDocker:     true, phpVersion: "8.2", phpVersionExplicit: true,
			selectedVersion: "6.7.0.0", selectedDeployment: shop.DeploymentNone, selectedCI: shop.CINone,
		}

		_, _, err := validateAndPreflight(t.Context(), &opts, releases, filteredVersions)
		assert.ErrorContains(t, err, "8.2")
		assert.ErrorContains(t, err, "--php-version")
	})
}
