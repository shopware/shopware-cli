package docker

import (
	"fmt"
	"strconv"
	"strings"
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
	// CABundlePath is the host path of a CA bundle that combines the image's
	// system CAs with the proxy's root CA. When set, it is mounted read-only
	// over the container's system trust store so everything there — PHP, curl
	// (e.g. Shopware's own APP_URL reachability self-call) and Node — trusts the
	// proxy's HTTPS certificates while still trusting the public internet.
	// Mounting the bare CA under /usr/local/share/ca-certificates is not enough:
	// the image runs as www-data and never runs update-ca-certificates.
	CABundlePath string
	// AdminWatchPort is the container port the admin watcher's dev server binds:
	// Vite (5173) on Shopware 6.7+, webpack-dev-server (8080) before, decided by
	// the ADMIN_VITE feature flag. The caller computes it (via
	// extension.AdminDevServerPort) and passes it in, so this package needs no
	// Shopware-version logic of its own.
	AdminWatchPort int
}

// containerCABundlePath is where the combined CA bundle is mounted inside the
// shop's containers: straight over the system trust store, so openssl, curl and
// PHP pick it up with no update-ca-certificates step (which the www-data image
// user could not run anyway). NODE_EXTRA_CA_CERTS points Node at the same file.
const containerCABundlePath = "/etc/ssl/certs/ca-certificates.crt"

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
func publishOrRoute(svc *composeService, p *ProxyOptions, serviceName string, hostPorts []string, routes ...proxyRoute) {
	if p == nil {
		if len(hostPorts) > 0 {
			svc.Ports = hostPorts
		}
		return
	}

	addProxyRouting(svc, p, serviceName, routes...)
}

// publishOrRouteService wires a catalog-defined service for the current mode:
// plain mode publishes the service's keyed host ports, proxy mode routes its
// endpoint subdomains. Web uses publishOrRoute directly because its routes are
// custom (see webProxyRoutes).
func publishOrRouteService(svc *composeService, p *ProxyOptions, def *ServiceDefinition, opts *ComposeOptions) {
	publishOrRoute(svc, p, def.Name, opts.portBindings(def.portKeys()...), def.proxyRoutes()...)
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
		{subdomain: SubdomainAdminWatch, containerPort: p.AdminWatchPort},
		// The deprecated webpack storefront watcher runs two servers under one
		// hostname: the HTML proxy on websecure and the asset+HMR server on the
		// dedicated sfassets entrypoint.
		{subdomain: SubdomainStorefrontWatch, containerPort: storefrontProxyPort},
		{subdomain: SubdomainStorefrontWatch, containerPort: storefrontAssetsPort, entrypoint: storefrontAssetsEntrypoint, nameSuffix: "storefront-watch-assets"},
		// /bundles/ (the ESM import-map modules) must come straight from the app
		// so they keep their JS Content-Type; the hot-proxy drops it and the
		// browser then rejects the module. Higher priority than the route above.
		{subdomain: SubdomainStorefrontWatch, pathPrefix: "/bundles/", containerPort: 8000, nameSuffix: "storefront-watch-bundles"},
	}
}

// proxyServiceAlias is the project-unique name a proxied service advertises on
// the shared network, so parallel projects don't collide on the bare service
// name there (issue #1484). Matches the Traefik router prefix (dots to dashes).
func proxyServiceAlias(hostname, serviceName string) string {
	return strings.ReplaceAll(hostname, ".", "-") + "-" + serviceName
}

// proxyServiceHost is the host web/console use to reach serviceName: the bare
// name in plain mode, the project-unique alias in proxy mode (issue #1484).
func proxyServiceHost(px *ProxyOptions, serviceName string) string {
	if px == nil {
		return serviceName
	}

	return proxyServiceAlias(px.Hostname, serviceName)
}

// proxyNetworks attaches a service to the default and shared proxy networks;
// alias, when set, is the project-unique name it advertises on the shared one.
func proxyNetworks(networkName, alias string) yamlMap[composeServiceNetwork] {
	shared := composeServiceNetwork{}
	if alias != "" {
		shared.Aliases = []string{alias}
	}

	return yamlMap[composeServiceNetwork]{}.
		set("default", composeServiceNetwork{}).
		set(networkName, shared)
}

// addProxyRouting joins serviceName to the shared proxy network and adds a
// Traefik router per route. buildCompose calls it in proxy mode instead of
// publishing fixed host ports.
func addProxyRouting(svc *composeService, p *ProxyOptions, serviceName string, routes ...proxyRoute) {
	// Router/service names must be unique across every project sharing the one
	// Traefik instance, so they are prefixed with the project's own hostname
	// (dots replaced, since Traefik router names must be alphanumeric).
	routerPrefix := proxyServiceAlias(p.Hostname, serviceName)

	// The service advertises that same alias on the shared network, so internal
	// calls resolve to it and not to a parallel project's service (#1484).
	svc.Networks = proxyNetworks(p.NetworkName, routerPrefix)

	labels := yamlMap[string]{}.
		set("traefik.enable", "true").
		set("traefik.docker.network", p.NetworkName)

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

		labels = labels.
			set(fmt.Sprintf("traefik.http.routers.%s.rule", router), rule).
			set(fmt.Sprintf("traefik.http.routers.%s.entrypoints", router), entrypoint).
			set(fmt.Sprintf("traefik.http.routers.%s.tls", router), "true").
			set(fmt.Sprintf("traefik.http.routers.%s.service", router), router).
			set(fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", router), strconv.Itoa(route.containerPort))
	}

	svc.Labels = labels
}

// addVolumes attaches a service's volume list built from base mounts, plus the
// read-only combined CA bundle mounted over the system trust store when in
// proxy mode — so code in the container (PHP, curl, Node) trusts the proxy's
// HTTPS certificates for self-calls to APP_URL while still trusting public CAs.
// Mirrors publishOrRoute, keeping the proxy-specific mount out of buildCompose.
func addVolumes(svc *composeService, p *ProxyOptions, base ...string) {
	vols := base
	if p != nil && p.CABundlePath != "" {
		vols = append(vols, p.CABundlePath+":"+containerCABundlePath+":ro")
	}

	svc.Volumes = vols
}
