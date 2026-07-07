// Package proxy implements the local reverse proxy for routing stable
// per-project hostnames (e.g. https://my-shop.127.0.0.1.sslip.io) to local
// Shopware instances. It manages a shared Traefik container, a local
// certificate authority for trusted HTTPS and the per-project Docker Compose
// integration.
package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// NetworkName is the shared Docker network joined by the proxy and all instances.
	NetworkName = "shopware-cli"
	// ContainerName is the name of the shared Traefik container.
	ContainerName = "shopware-cli-proxy"
	// TraefikImage is the Traefik image used for the shared proxy. Traefik
	// v3.6+ is required, older versions pin Docker API version 1.24 which
	// current Docker daemons reject.
	TraefikImage = "traefik:v3.6"
	// DefaultDomain is the base domain under which instances get their hostname.
	// sslip.io resolves <anything>.127.0.0.1.sslip.io to 127.0.0.1 without any
	// local DNS setup on macOS, Linux and Windows.
	DefaultDomain = "127.0.0.1.sslip.io"
	// HostLabel marks a proxied container and stores its assigned hostname.
	HostLabel = "com.shopware-cli.proxy.host"
)

// Dir returns the directory holding all proxy state (CA, certificates,
// Traefik configuration). It can be overridden with SHOPWARE_CLI_PROXY_DIR.
func Dir() (string, error) {
	if dir := os.Getenv("SHOPWARE_CLI_PROXY_DIR"); dir != "" {
		return dir, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine user config directory: %w", err)
	}

	return filepath.Join(configDir, "shopware-cli", "proxy"), nil
}

var invalidNameChars = regexp.MustCompile(`[^a-z0-9-]`)

// SanitizeName turns an arbitrary string (usually the project folder name)
// into a valid DNS label usable as subdomain and Traefik router name.
func SanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	name = invalidNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	if name == "" {
		name = "shopware"
	}

	return name
}
