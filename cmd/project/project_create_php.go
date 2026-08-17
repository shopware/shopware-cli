package project

import (
	"context"
	"fmt"
	"os"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

// discoverPHPInstallations and unusablePHPBinaryEnv are seams for tests, which
// must not depend on the PHP versions installed on the developer machine.
var (
	discoverPHPInstallations = system.DiscoverPHPInstallations
	unusablePHPBinaryEnv     = system.UnusablePHPBinaryEnv
)

// resolveLocalPHP sets opts.phpBinary to the local PHP used to create the
// project. Precedence: --php-version, then the interactive form's choice, then a
// compatible PHP_BINARY, then the compatible PATH default. When nothing usable is
// discovered it stays empty and the dependency validation reports the error.
func resolveLocalPHP(ctx context.Context, opts *createOptions, phpConstraint *shop.PHPConstraint) error {
	// Discovery omits an unusable PHP_BINARY, so check it separately rather than
	// silently replacing it with another PHP.
	if err := unusablePHPBinaryEnv(ctx); err != nil {
		return err
	}

	installations := discoverPHPInstallations(ctx)

	if opts.phpVersionExplicit {
		installation := system.FindPHPByVersionPin(installations, opts.phpVersion)
		if installation == nil {
			return &system.PHPVersionNotFoundError{Pin: opts.phpVersion, Installations: installations}
		}

		if phpConstraint != nil && !phpConstraint.Check(installation.Version) {
			return fmt.Errorf("the requested PHP %s does not satisfy the PHP constraint %s of the selected Shopware version; pass --php-version with a matching version", opts.phpVersion, phpConstraint)
		}

		opts.setPHP(*installation)
		return nil
	}

	compatible := system.FilterCompatiblePHP(installations, phpConstraint)

	if len(compatible) == 0 {
		// Leave opts.phpBinary empty so the dependency validation reports the
		// error, but show what was found so the user sees which versions exist.
		if len(installations) > 0 {
			fmt.Fprintln(os.Stderr, renderDiscoveredPHP(installations, phpConstraint))
		}
		return nil
	}

	// Only fill in what the form left empty: overwriting would discard the PHP the
	// user picked in the wizard and confirmed in its summary.
	if opts.interactive {
		if opts.phpBinary != "" {
			return nil
		}

		if preferred := system.PreferredPHPInstallation(compatible); preferred != nil {
			opts.setPHP(*preferred)
		}
		return nil
	}

	fallback := system.FindPHPBySource(compatible, system.PHPSourceEnv)
	if fallback == nil {
		fallback = system.DefaultPHPInstallation(compatible)
	}
	if fallback != nil {
		logging.FromContext(ctx).Infof("Using PHP %s from %s (%s); use --php-version to override", fallback.Version, fallback.Source, fallback.Binary)
		opts.setPHP(*fallback)
		return nil
	}

	var found []string
	for _, installation := range compatible {
		found = append(found, installation.String())
	}

	return fmt.Errorf("neither PHP_BINARY nor the php found in PATH satisfies the PHP constraint %s of the selected Shopware version; select one of the compatible installations with --php-version: %s", phpConstraint, strings.Join(found, ", "))
}

// compatiblePHPFor returns the discovered PHP installations that satisfy the
// PHP constraint of the given Shopware version, newest first.
func compatiblePHPFor(ctx context.Context, releases []repository.Version, selectedVersion string, filteredVersions []*version.Version) []system.PHPInstallation {
	return filterCompatiblePHPFor(discoverPHPInstallations(ctx), releases, selectedVersion, filteredVersions)
}

// filterCompatiblePHPFor is separate from discovery so the form can refilter an
// already discovered set when the version changes, without probing again.
func filterCompatiblePHPFor(installations []system.PHPInstallation, releases []repository.Version, selectedVersion string, filteredVersions []*version.Version) []system.PHPInstallation {
	if _, err := shop.ResolveInstallVersion(selectedVersion, filteredVersions); err != nil {
		return nil
	}

	return system.FilterCompatiblePHP(installations, phpConstraintFor(releases, selectedVersion, filteredVersions))
}

// phpConstraintFor returns the PHP constraint of the given Shopware version, or
// nil when it cannot be resolved (which matches every version).
func phpConstraintFor(releases []repository.Version, selectedVersion string, filteredVersions []*version.Version) *shop.PHPConstraint {
	chosenVersion, err := shop.ResolveInstallVersion(selectedVersion, filteredVersions)
	if err != nil {
		return nil
	}

	return shop.PHPConstraintForShopwareVersion(releases, chosenVersion)
}

// shouldPromptPHPSelection reports whether the form asks which PHP to use: only
// when there is an actual choice. Must be decidable without the select field's
// async OptionsFunc, since huh evaluates hide funcs during navigation.
func shouldPromptPHPSelection(candidates int) bool {
	return candidates > 1
}

// highestOrEmpty returns the last entry of a SupportedPHPVersions-ordered list
// (lowest to highest), or an empty string when there is none.
func highestOrEmpty(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	return versions[len(versions)-1]
}

func phpVersionOptions(versions []string) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(versions))
	for _, phpVersion := range versions {
		options = append(options, huh.NewOption("PHP "+phpVersion, phpVersion))
	}
	return options
}

func phpInstallationOptions(installations []system.PHPInstallation) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(installations))
	for _, installation := range installations {
		label := installation.String()
		if installation.Source != "" {
			label += " (" + installation.Source + ")"
		}
		options = append(options, huh.NewOption(label, installation.Binary))
	}
	return options
}

// renderDiscoveredPHP renders the PHP installations found on this machine when
// none of them satisfies the PHP constraint of the selected Shopware version.
func renderDiscoveredPHP(installations []system.PHPInstallation, phpConstraint *shop.PHPConstraint) string {
	var b strings.Builder

	title := "Discovered PHP installations"
	if constraint := phpConstraint.String(); constraint != "" {
		title += fmt.Sprintf(" (none satisfies %s)", constraint)
	}
	b.WriteString(tui.RedText.Bold(true).Render(title))
	b.WriteString("\n\n")

	cross := tui.RedText.Render("✗")
	for _, installation := range installations {
		fmt.Fprintf(&b, "  %s %s %s\n", cross, tui.BoldText.Render(installation.String()), tui.DimText.Render("("+installation.Source+")"))
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BlueColor).
		Padding(1, 2).
		Render(strings.TrimRight(b.String(), "\n"))
}
