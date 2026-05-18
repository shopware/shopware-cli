package project

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

func validateAndPreflight(ctx context.Context, opts *createOptions, releases []packagist.ComposerPackageVersion, filteredVersions []*version.Version) (string, *packagist.PHPConstraint, error) {
	chosenVersion := resolveVersion(opts.selectedVersion, filteredVersions)
	if chosenVersion == "" {
		return "", nil, fmt.Errorf("cannot find version %s", opts.selectedVersion)
	}

	phpConstraint := packagist.PHPConstraintForShopwareVersion(releases, chosenVersion)

	missingDeps := system.CheckProjectDependencies(ctx, opts.useDocker, phpConstraint)

	validDeployments := map[string]bool{
		packagist.DeploymentNone:         true,
		packagist.DeploymentDeployer:     true,
		packagist.DeploymentPlatformSH:   true,
		packagist.DeploymentShopwarePaaS: true,
	}
	if !validDeployments[opts.selectedDeployment] {
		return "", nil, fmt.Errorf("invalid deployment method: %s. Valid options: none, deployer, platformsh, shopware-paas", opts.selectedDeployment)
	}

	validCISystems := map[string]bool{
		ciNone:   true,
		ciGitHub: true,
		ciGitLab: true,
	}
	if !validCISystems[opts.selectedCI] {
		return "", nil, fmt.Errorf("invalid CI system: %s. Valid options: none, github, gitlab", opts.selectedCI)
	}

	if len(missingDeps) > 0 {
		fmt.Fprintln(os.Stderr, system.RenderMissingDependencies(opts.useDocker, missingDeps))
		return "", nil, fmt.Errorf("missing required dependencies")
	}

	if err := checkSecurityAdvisories(ctx, opts, chosenVersion); err != nil {
		return "", nil, err
	}

	if err := checkIncompatibilities(ctx, opts); err != nil {
		return "", nil, err
	}

	if _, err := os.Stat(opts.projectFolder); err == nil {
		empty, err := system.IsDirEmpty(opts.projectFolder)
		if err != nil {
			return "", nil, err
		}

		if !empty {
			return "", nil, fmt.Errorf("the folder %s exists already and is not empty", opts.projectFolder)
		}
	}

	// @todo: it's broken in paas deployments, the paas recipe configures Elasticsearch and it's difficult to do it only when elasticsearch is available.
	if opts.selectedDeployment == packagist.DeploymentShopwarePaaS {
		opts.withElasticsearch = true
	}

	return chosenVersion, phpConstraint, nil
}

func checkSecurityAdvisories(ctx context.Context, opts *createOptions, chosenVersion string) error {
	advisories, err := packagist.GetShopwareSecurityAdvisories(ctx)
	if err != nil {
		logging.FromContext(ctx).Warnf("Could not fetch security advisories: %v", err)
	}

	matchingAdvisories := packagist.FilterAdvisoriesForVersion(advisories, chosenVersion)
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

func renderSecurityAdvisories(chosenVersion string, advisories []packagist.SecurityAdvisory) string {
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

func resolveVersion(selectedVersion string, filteredVersions []*version.Version) string {
	if selectedVersion == versionLatest {
		// pick the most recent stable (non-RC) version
		for _, v := range filteredVersions {
			vs := v.String()
			if !strings.Contains(strings.ToLower(vs), "rc") {
				return vs
			}
		}
		// if no stable found, fall back to top entry
		if len(filteredVersions) > 0 {
			return filteredVersions[0].String()
		}
		return ""
	}

	if strings.HasPrefix(selectedVersion, "dev-") {
		return selectedVersion
	}

	for _, release := range filteredVersions {
		if release.String() == selectedVersion {
			return release.String()
		}
	}

	return ""
}

func filterInstallVersions(releases []packagist.ComposerPackageVersion) []*version.Version {
	filteredVersions := make([]*version.Version, 0)
	constraint, _ := version.NewConstraint(">=6.4.18.0")

	for _, release := range releases {
		if strings.HasPrefix(release.Version, "dev-") {
			continue
		}

		parsed, err := version.NewVersion(release.Version)
		if err != nil {
			continue
		}

		if constraint.Check(parsed) {
			filteredVersions = append(filteredVersions, parsed)
		}
	}

	sort.Sort(sort.Reverse(version.Collection(filteredVersions)))

	for i, v := range filteredVersions {
		filteredVersions[i], _ = version.NewVersion(strings.TrimPrefix(v.String(), "v"))
	}

	return filteredVersions
}
