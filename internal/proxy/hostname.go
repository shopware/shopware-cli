package proxy

import (
	"fmt"
	"net"
	"net/url"
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
	// map them to dashes (matching localDomainHostname in project create) before
	// validating and using the name as a hostname label.
	name := strings.ReplaceAll(filepath.Base(projectRoot), "_", "-")
	if err := system.ValidateDockerComposeName(name); err != nil {
		return "", fmt.Errorf("cannot derive a hostname from directory name %q: %w", filepath.Base(projectRoot), err)
	}

	return fmt.Sprintf("%s.%s", name, baseDomain), nil
}
