package proxy

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"

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

	name := filepath.Base(projectRoot)
	if err := system.ValidateDockerComposeName(name); err != nil {
		return "", fmt.Errorf("cannot derive a hostname from directory name %q: %w", name, err)
	}

	return fmt.Sprintf("%s.%s", name, baseDomain), nil
}
