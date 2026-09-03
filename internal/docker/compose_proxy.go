package docker

import (
	"fmt"
	"strconv"
	"strings"
)

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

// routeHostname returns the full hostname for a route, e.g.
// "admin-watch.my-shop.shopware.local" or, for the root route,
// "my-shop.shopware.local".
func routeHostname(p *Proxy, r proxyRoute) string {
	if r.subdomain == "" {
		return p.Hostname
	}

	return fmt.Sprintf("%s.%s", r.subdomain, p.Hostname)
}

// publishOrRoute wires a built service for the current mode. In proxy mode a
// service with routes joins the shared network and gets a Traefik router per
// route and publishes no host ports. Otherwise its endpoints are published on
// the host, honoring port overrides; a service without routes (the database)
// is published the same way in both modes.
func publishOrRoute(spec *composeService, e *Environment, svc service) {
	if routes := e.routes(svc); len(routes) > 0 {
		addProxyRouting(spec, e.proxy, svc.Name, routes...)
		return
	}

	if bindings := portBindings(svc, e.ports(svc.Name)); len(bindings) > 0 {
		spec.Ports = bindings
	}
}

// webProxyRoutes returns the web service's routes in proxy mode: the shop root,
// the admin watcher, and the deprecated webpack storefront watcher's three
// endpoints (HTML proxy, asset/HMR server, and /bundles/ served from the app).
// It returns nil for a nil (plain-mode) proxy.
func webProxyRoutes(p *Proxy) []proxyRoute {
	if p == nil {
		return nil
	}

	return []proxyRoute{
		{subdomain: "", containerPort: 8000},
		{subdomain: subdomainAdminWatch, containerPort: p.AdminWatchPort},
		// The deprecated webpack storefront watcher runs two servers under one
		// hostname: the HTML proxy on websecure and the asset+HMR server on the
		// dedicated sfassets entrypoint.
		{subdomain: subdomainStorefrontWatch, containerPort: storefrontProxyPort},
		{subdomain: subdomainStorefrontWatch, containerPort: storefrontAssetsPort, entrypoint: storefrontAssetsEntrypoint, nameSuffix: "storefront-watch-assets"},
		// /bundles/ (the ESM import-map modules) must come straight from the app
		// so they keep their JS Content-Type; the hot-proxy drops it and the
		// browser then rejects the module. Higher priority than the route above.
		{subdomain: subdomainStorefrontWatch, pathPrefix: "/bundles/", containerPort: 8000, nameSuffix: "storefront-watch-bundles"},
	}
}

// proxyServiceAlias is the project-unique name a proxied service advertises on
// the shared network, so parallel projects don't collide on the bare service
// name there (issue #1484). Matches the Traefik router prefix (dots to dashes).
func proxyServiceAlias(hostname, serviceName string) string {
	return strings.ReplaceAll(hostname, ".", "-") + "-" + serviceName
}

// serviceHost is the host the PHP containers use to reach a routed service:
// the bare compose name in plain mode, the project-unique alias it advertises
// on the shared proxy network in proxy mode (issue #1484). It is only meant
// for services that are routed (mailer, queue, search, storage); unrouted
// services (database, cache, storage-init) stay on the project's own network,
// where the bare name cannot collide, and are addressed by it directly.
func (e *Environment) serviceHost(name string) string {
	if e.proxy == nil {
		return name
	}

	return proxyServiceAlias(e.proxy.Hostname, name)
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
// Traefik router per route.
func addProxyRouting(spec *composeService, p *Proxy, serviceName string, routes ...proxyRoute) {
	// Router/service names must be unique across every project sharing the one
	// Traefik instance, so they are prefixed with the project's own hostname
	// (dots replaced, since Traefik router names must be alphanumeric).
	routerPrefix := proxyServiceAlias(p.Hostname, serviceName)

	// The service advertises that same alias on the shared network, so internal
	// calls resolve to it and not to a parallel project's service (#1484).
	spec.Networks = proxyNetworks(p.NetworkName, routerPrefix)

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

		rule := fmt.Sprintf("Host(`%s`)", routeHostname(p, route))
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

	spec.Labels = labels
}
