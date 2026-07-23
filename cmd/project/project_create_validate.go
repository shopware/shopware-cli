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

func validateAndPreflight(ctx context.Context, opts *createOptions, releases []repository.Version, filteredVersions []*version.Version) (string, *shop.PHPConstraint, error) {
	chosenVersion, err := shop.ResolveInstallVersion(opts.selectedVersion, filteredVersions)
	if err != nil {
		return "", nil, err
	}

	phpConstraint := shop.PHPConstraintForShopwareVersion(releases, chosenVersion)

	scaffold := newShopwareProjectScaffold(opts, chosenVersion)
	scaffold.Normalize()
	if err := scaffold.Validate(); err != nil {
		return "", nil, err
	}
	opts.selectedDeployment = scaffold.DeploymentMethod
	opts.selectedCI = scaffold.CISystem
	opts.withElasticsearch = scaffold.UseElasticsearch

	dockerHint := "re-run with " + tui.BoldText.Render("--docker")
	if err := system.ValidateProjectDependencies(ctx, opts.useDocker, phpConstraint, "create a Shopware project", dockerHint); err != nil {
		return "", nil, err
	}

	if err := checkSecurityAdvisories(ctx, opts, chosenVersion); err != nil {
		return "", nil, err
	}

	if err := checkIncompatibilities(ctx, opts); err != nil {
		return "", nil, err
	}

	return chosenVersion, phpConstraint, nil
}

func checkSecurityAdvisories(ctx context.Context, opts *createOptions, chosenVersion string) error {
	advisories, err := repository.New(repository.PackagistURL, nil).GetSecurityAdvisories(ctx, []string{"shopware/core"})
	if err != nil {
		logging.FromContext(ctx).Warnf("Could not fetch security advisories: %v", err)
	}

	// affectedByConstraint reports whether the chosen version satisfies an
	// advisory's affectedVersions branch. go-composer splits the OR/AND
	// branches; go-version evaluates each one.
	affectedByConstraint := func(constraint, ver string) bool {
		v, err := version.NewVersion(strings.TrimPrefix(ver, "v"))
		if err != nil {
			return false
		}
		cs, err := version.NewConstraint(constraint)
		if err != nil {
			return false
		}
		return cs.Check(v)
	}

	matchingAdvisories := advisories.AffectingPackage("shopware/core", chosenVersion, affectedByConstraint)
	if len(matchingAdvisories) == 0 {
		return nil
	}

	fmt.Fprintln(os.Stderr, renderSecurityAdvisories(chosenVersion, matchingAdvisories))

	if opts.interactive {
		var continueAnyway string
		if err := huh.NewForm(huh.NewGroup(
			tui.NewYesNo().
				Title(fmt.Sprintf("Shopware %s is affected by %d known security %s", chosenVersion, len(matchingAdvisories), pluralize(len(matchingAdvisories), "advisory", "advisories"))).
				Description("Continuing will disable composer's audit blocking (--no-audit) so installation can proceed. If you continue, we strongly recommend installing the Shopware Security plugin (https://store.shopware.com/en/swag136939272659f/shopware-6-security-plugin.html) which backports security fixes to older versions. Do you want to continue anyway?").
				Value(&continueAnyway),
		)).Run(); err != nil {
			return err
		}

		if continueAnyway == tui.No {
			return fmt.Errorf("project creation cancelled")
		}

		opts.noAudit = true
		return nil
	}

	if !opts.noAudit {
		return fmt.Errorf("shopware %s is affected by known security advisories; re-run with --no-audit to proceed. We strongly recommend installing the Shopware Security plugin (https://store.shopware.com/en/swag136939272659f/shopware-6-security-plugin.html) which backports security fixes to older versions", chosenVersion)
	}

	return nil
}

func checkIncompatibilities(ctx context.Context, opts *createOptions) error {
	incompatibilities := system.CheckIncompatibilities(opts.useDocker, opts.projectFolder)

	for _, incompatibility := range incompatibilities {
		if opts.interactive {
			var continueAnyway string
			if err := huh.NewForm(huh.NewGroup(
				tui.NewYesNo().
					Title(incompatibility.Title).
					Description(fmt.Sprintf("%s. Do you want to continue anyway?", incompatibility.Description)).
					Value(&continueAnyway),
			)).Run(); err != nil {
				return err
			}

			if continueAnyway == tui.No {
				return fmt.Errorf("project creation cancelled")
			}
		} else {
			logging.FromContext(ctx).Warnf("%s. %s", incompatibility.Title, incompatibility.Description)
		}
	}

	return nil
}

func renderSecurityAdvisories(chosenVersion string, advisories []repository.SecurityAdvisory) string {
	var b strings.Builder

	b.WriteString(tui.RedText.Bold(true).Render(fmt.Sprintf("Security Advisories for Shopware %s", chosenVersion)))
	b.WriteString("\n\n")

	warn := tui.YellowText.Render("⚠")
	for _, a := range advisories {
		severity := strings.ToUpper(a.Severity)
		if severity == "" {
			severity = "UNKNOWN"
		}
		fmt.Fprintf(&b, "  %s [%s] %s\n", warn, tui.BoldText.Render(severity), a.Title)
		if a.CVE != "" {
			fmt.Fprintf(&b, "    %s: %s\n", tui.DimText.Render("CVE"), a.CVE)
		}
		if a.Link != "" {
			fmt.Fprintf(&b, "    %s\n", tui.BlueText.Render(a.Link))
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BlueColor).
		Padding(1, 2).
		Render(strings.TrimRight(b.String(), "\n"))
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
