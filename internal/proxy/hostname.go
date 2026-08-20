package proxy

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// ProjectHostname derives the hostname a project should be reachable at
// through the shared proxy. If cfg.URL is explicitly set in
// .shopware-project.yml, its host is used as an override — unless it is an
// IP address or localhost, which is what freshly created projects point at
// (http://127.0.0.1:8000) and never a usable proxy hostname. Otherwise the
// hostname is derived from the project directory name.
func ProjectHostname(projectRoot string, cfg *shop.Config, baseDomain string) (string, error) {
	if cfg != nil && cfg.URL != "" {
		parsed, err := url.Parse(cfg.URL)
		if err != nil {
			return "", fmt.Errorf("parsing configured url %q: %w", cfg.URL, err)
		}

		if host := parsed.Hostname(); host != "" && host != "localhost" && net.ParseIP(host) == nil {
			return host, nil
		}
	}

	// Docker Compose project names allow underscores, but DNS labels do not, so
	// map them to dashes (matching LocalDomainHostname) before validating and
	// using the name as a hostname label.
	name := strings.ReplaceAll(filepath.Base(projectRoot), "_", "-")
	if err := system.ValidateDockerComposeName(name); err != nil {
		return "", fmt.Errorf("cannot derive a hostname from directory name %q: %w", filepath.Base(projectRoot), err)
	}

	return fmt.Sprintf("%s.%s", name, baseDomain), nil
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
