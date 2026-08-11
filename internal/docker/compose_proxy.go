package docker

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProxyOptions carries the data needed to route a project's services through
// the shared shopware-cli proxy by hostname instead of fixed host ports. It is
// set on ComposeOptions.Proxy to switch buildCompose into proxy mode.
type ProxyOptions struct {
	// Hostname is the project's resolved root hostname, e.g.
	// "my-shop.shopware.local". Every other route is a subdomain of it (e.g.
	// "admin-watch.my-shop.shopware.local").
	Hostname string
	// NetworkName is the external Docker network shared with Traefik.
	NetworkName string
	// CAPath is the host path of the proxy's root CA certificate. When set,
	// it is mounted read-only into the shop's containers so code running
	// there (Node via NODE_EXTRA_CA_CERTS, and PHP once the image trusts the
	// anchor) accepts the proxy's HTTPS certificates — needed when the shop
	// calls its own APP_URL over HTTPS (and, as a side effect, a sibling's).
	CAPath string
	// AppURL, when set, pins APP_URL as a real container env var on the web and
	// background services. It must be set here (not only in .env.local) because
	// the container is created before proxy up rewrites .env.local, and a real
	// env var reliably beats both the env_file and the image default — otherwise
	// PHP renders absolute asset URLs (e.g. the storefront import map) with the
	// stale image APP_URL.
	AppURL string
	// AdminWatchPort is the container port the admin watcher's dev server binds:
	// Vite (5173) on Shopware 6.7+, webpack-dev-server (8080) before, decided by
	// the ADMIN_VITE feature flag. The caller computes it (via
	// extension.AdminDevServerPort) and passes it in, so this package needs no
	// Shopware-version logic of its own.
	AdminWatchPort int
}

// containerCAPath is where the proxy CA is mounted inside the shop's
// containers. /usr/local/share/ca-certificates is the standard anchor
// directory picked up by update-ca-certificates, and the file is also
// referenced directly by NODE_EXTRA_CA_CERTS.
const containerCAPath = "/usr/local/share/ca-certificates/shopware-cli-proxy.crt"

const (
	// storefrontProxyPort / storefrontAssetsPort are the two container ports the
	// deprecated webpack hot-proxy watcher listens on: the HTML proxy and the
	// asset+HMR server. They must match internal/extension/storefront_watch.go.
	storefrontProxyPort  = 9998
	storefrontAssetsPort = 8443
	// storefrontAssetsEntrypoint is the dedicated Traefik entrypoint the asset
	// server binds to, so it can share the storefront-watch hostname with the
	// HTML proxy while listening on a different port. Must match the
	// "sfassets" entrypoint defined in internal/proxy/traefik.go.
	storefrontAssetsEntrypoint = "sfassets"
)

// proxyRoute describes one HTTP endpoint of a service that gets routed
// through the shared proxy: its subdomain (empty means the project's root
// hostname) and the container port it is served on.
type proxyRoute struct {
	subdomain     string
	containerPort int
	// entrypoint is the Traefik entrypoint this route binds to; empty means
	// "websecure" (the shared HTTPS :443 port). The storefront asset/HMR server
	// uses a dedicated entrypoint so it can share the storefront-watch hostname
	// with the HTML proxy while listening on a different port.
	entrypoint string
	// nameSuffix disambiguates the Traefik router name when two routes share a
	// subdomain (the storefront HTML proxy and its asset server). Empty falls
	// back to the subdomain.
	nameSuffix string
	// pathPrefix, when set, narrows the route to requests under that path
	// (Traefik prioritizes longer rules, so a Host+PathPrefix route wins over a
	// bare Host route for matching requests). Used to serve /bundles/ static
	// assets straight from the app, bypassing the hot-proxy — which drops their
	// Content-Type and breaks ESM module scripts.
	pathPrefix string
}

// ProxiedServiceLabels maps compose service names to the human-readable label
// shown for their proxied subdomain link (e.g. in `project proxy list`). Only
// services with a web UI are listed; it lives here next to the routing that
// exposes those subdomains.
var ProxiedServiceLabels = map[string]string{
	"adminer":    "Adminer",
	"mailer":     "Mailpit",
	"lavinmq":    "Queue",
	"opensearch": "Search",
}

// hostname returns the full hostname for this route, e.g.
// "admin-watch.my-shop.shopware.local" or, for the root route,
// "my-shop.shopware.local".
func (p *ProxyOptions) hostname(r proxyRoute) string {
	if r.subdomain == "" {
		return p.Hostname
	}

	return fmt.Sprintf("%s.%s", r.subdomain, p.Hostname)
}

// publishOrRoute wires a service for the current mode: in plain mode it
// publishes the fixed host ports, in proxy mode it joins the shared network and
// adds a Traefik router per route (and never publishes host ports). Keeping the
// mode branch here keeps buildCompose readable.
func publishOrRoute(svc *yaml.Node, p *ProxyOptions, serviceName string, hostPorts []string, routes ...proxyRoute) {
	if p == nil {
		if len(hostPorts) > 0 {
			addKeyValueNode(svc, "ports", newSequenceNode(hostPorts...))
		}
		return
	}

	addProxyRouting(svc, p, serviceName, routes...)
}

// webProxyRoutes returns the web service's routes in proxy mode: the shop root,
// the admin watcher, and the deprecated webpack storefront watcher's three
// endpoints (HTML proxy, asset/HMR server, and /bundles/ served from the app).
// It returns nil for a nil (plain-mode) options value.
func webProxyRoutes(p *ProxyOptions) []proxyRoute {
	if p == nil {
		return nil
	}

	return []proxyRoute{
		{subdomain: "", containerPort: 8000},
		{subdomain: "admin-watch", containerPort: p.AdminWatchPort},
		// The deprecated webpack storefront watcher runs two servers under one
		// hostname: the HTML proxy on websecure and the asset+HMR server on the
		// dedicated sfassets entrypoint.
		{subdomain: "storefront-watch", containerPort: storefrontProxyPort},
		{subdomain: "storefront-watch", containerPort: storefrontAssetsPort, entrypoint: storefrontAssetsEntrypoint, nameSuffix: "storefront-watch-assets"},
		// /bundles/ (the ESM import-map modules) must come straight from the app
		// so they keep their JS Content-Type; the hot-proxy drops it and the
		// browser then rejects the module. Higher priority than the route above.
		{subdomain: "storefront-watch", pathPrefix: "/bundles/", containerPort: 8000, nameSuffix: "storefront-watch-bundles"},
	}
}

// addProxyRouting joins serviceName to the shared proxy network and adds a
// Traefik router per route. buildCompose calls it in proxy mode instead of
// publishing fixed host ports.
func addProxyRouting(svc *yaml.Node, p *ProxyOptions, serviceName string, routes ...proxyRoute) {
	addKeyValueNode(svc, "networks", newSequenceNode("default", p.NetworkName))

	labels := newMappingNode()
	addKeyValue(labels, "traefik.enable", "true")
	addKeyValue(labels, "traefik.docker.network", p.NetworkName)

	// Router/service names must be unique across every project sharing the one
	// Traefik instance, so they are prefixed with the project's own hostname
	// (dots replaced, since Traefik router names must be alphanumeric).
	routerPrefix := strings.ReplaceAll(p.Hostname, ".", "-") + "-" + serviceName

	for _, route := range routes {
		name := route.nameSuffix
		if name == "" {
			name = route.subdomain
		}

		router := routerPrefix
		if name != "" && name != serviceName {
			router = router + "-" + name
		}

		entrypoint := route.entrypoint
		if entrypoint == "" {
			entrypoint = "websecure"
		}

		rule := fmt.Sprintf("Host(`%s`)", p.hostname(route))
		if route.pathPrefix != "" {
			rule += fmt.Sprintf(" && PathPrefix(`%s`)", route.pathPrefix)
		}

		addKeyValue(labels, fmt.Sprintf("traefik.http.routers.%s.rule", router), rule)
		addKeyValue(labels, fmt.Sprintf("traefik.http.routers.%s.entrypoints", router), entrypoint)
		addKeyValue(labels, fmt.Sprintf("traefik.http.routers.%s.tls", router), "true")
		addKeyValue(labels, fmt.Sprintf("traefik.http.routers.%s.service", router), router)
		addKeyValue(labels, fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", router), strconv.Itoa(route.containerPort))
	}

	addKeyValueNode(svc, "labels", labels)
}

// addVolumes attaches a service's volume list built from base mounts, plus the
// read-only proxy CA mount when in proxy mode with a CA path — so code in the
// container trusts the proxy's HTTPS certificates for self-calls to APP_URL.
// Mirrors publishOrRoute, keeping the proxy-specific mount out of buildCompose.
func addVolumes(svc *yaml.Node, p *ProxyOptions, base ...string) {
	vols := newSequenceNode(base...)
	if p != nil && p.CAPath != "" {
		vols.Content = append(vols.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: p.CAPath + ":" + containerCAPath + ":ro", Tag: "!!str"})
	}

	addKeyValueNode(svc, "volumes", vols)
}
