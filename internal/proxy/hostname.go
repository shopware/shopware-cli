package proxy

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
)

// ProjectHostname derives the hostname a project should be reachable at
// through the shared proxy. If a url is explicitly set in
// .config/shopware-project.yml, its host is used as an override — unless it is an
// IP address or localhost, which is what freshly created projects point at
// (http://127.0.0.1:8000) and never a usable proxy hostname. Otherwise the
// hostname is derived from the project directory name.
func ProjectHostname(projectRoot string, cfg *shop.Config, baseDomain string) (string, error) {
	if configured := cfg.EffectiveURL(); configured != "" {
		parsed, err := url.Parse(configured)
		if err != nil {
			return "", fmt.Errorf("parsing configured url %q: %w", configured, err)
		}

		if host := parsed.Hostname(); host != "" && host != "localhost" && net.ParseIP(host) == nil {
			if err := ValidateDomain(host); err != nil {
				return "", fmt.Errorf("the configured url has an invalid hostname: %w", err)
			}
			return host, nil
		}
	}

	// The directory name becomes a DNS label: underscores (valid in a
	// directory name, not in DNS) become dashes and it is lowercased, matching
	// LocalDomainHostname (used by `create --local-domain`) — a directory like
	// "MyShop" must not dead-end here just because that path accepts it.
	name := strings.ToLower(strings.ReplaceAll(filepath.Base(projectRoot), "_", "-"))
	hostname := fmt.Sprintf("%s.%s", name, baseDomain)
	if err := ValidateDomain(hostname); err != nil {
		return "", fmt.Errorf("cannot derive a hostname from directory name %q: %w; rename the directory, or set an explicit \"url\" in the project config", filepath.Base(projectRoot), err)
	}

	return hostname, nil
}

// LocalDomainHostname returns the stable proxy hostname for a project name,
// e.g. "my-shop.shopware.local". The current directory ("" or ".") resolves to
// the working directory's name so it never yields a malformed "..<domain>"
// hostname. Underscores (valid in a project name but not a DNS label) become
// dashes and the label is lowercased, matching Docker/DNS.
func LocalDomainHostname(name, baseDomain string) string {
	label := filepath.Base(name)
	if label == "." || label == "" || label == string(filepath.Separator) {
		if wd, err := os.Getwd(); err == nil {
			label = filepath.Base(wd)
		}
	}

	label = strings.ToLower(strings.ReplaceAll(label, "_", "-"))

	return label + "." + baseDomain
}

// BaseDomain returns the machine-wide proxy base domain from settings, falling
// back to the default when no settings are stored yet.
func BaseDomain() string {
	if s, err := LoadSettings(); err == nil {
		return s.BaseDomain()
	}

	return DefaultDomain
}
