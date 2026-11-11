package docker

import (
	"fmt"
	"slices"
	"strconv"
)

// customRoutes holds project-specific proxy routes. The web service needs them
// because its watcher ports and routes depend on project settings.
var customRoutes = map[string]func(*Environment) []proxyRoute{
	ServiceWeb: webProxyRoutes,
}

// routes returns the proxy routes of a service in the environment's mode: nil
// in fixed-port mode, otherwise the custom routes when the service defines
// them or one route per endpoint with a subdomain. A service with routes
// publishes no host ports; this is the single predicate the generator and the
// port probe share.
func (e *Environment) routes(svc service) []proxyRoute {
	if e.proxy == nil {
		return nil
	}

	return serviceRoutes(svc, e)
}

// serviceRoutes returns custom routes when the service has them and otherwise
// derives routes from its endpoints.
func serviceRoutes(svc service, e *Environment) []proxyRoute {
	if custom, ok := customRoutes[svc.Name]; ok {
		return custom(e)
	}

	var routes []proxyRoute
	for _, ep := range svc.Endpoints {
		if ep.Subdomain != "" {
			routes = append(routes, proxyRoute{subdomain: ep.Subdomain, containerPort: ep.ContainerPort})
		}
	}

	return routes
}

// RoutedSubdomains returns every proxy subdomain the environment's services
// are routed at, in catalog order. The web service contributes the root
// hostname ("") plus the watcher subdomains. It is independent of the run
// mode so the browser-facing hostnames can be listed before the proxy is up.
func (e *Environment) RoutedSubdomains() []string {
	var subs []string
	for _, svc := range e.activeServices() {
		subs = append(subs, serviceSubdomains(svc, e)...)
	}

	return subs
}

// serviceSubdomains returns the distinct proxy subdomains the service is
// routed at, in route order; an empty entry denotes the project's root
// hostname.
func serviceSubdomains(svc service, e *Environment) []string {
	// Web routes depend on Vite mode, so hostname discovery passes that state
	// to the route builder. A placeholder proxy enables route generation before
	// proxy setup. Only the route subdomains are used here.
	routeEnvironment := &Environment{
		proxy:              &Proxy{},
		storefrontUsesVite: e.storefrontUsesVite,
	}

	var subs []string
	for _, route := range serviceRoutes(svc, routeEnvironment) {
		if !slices.Contains(subs, route.subdomain) {
			subs = append(subs, route.subdomain)
		}
	}

	return subs
}

// portBindings renders the compose "ports" entries of the service's endpoints
// for plain mode, honoring port overrides and skipping endpoints that are
// disabled or never published. An endpoint without a fixed host port is
// published on a random one.
func portBindings(svc service, ports Ports) []string {
	bindings := make([]string, 0, len(svc.Endpoints))
	for _, ep := range svc.Endpoints {
		if !ep.published(ports) {
			continue
		}

		host := ""
		if hostPort := ep.hostPort(ports); hostPort > 0 {
			host = strconv.Itoa(hostPort)
		}

		switch {
		case ep.Loopback:
			bindings = append(bindings, fmt.Sprintf("127.0.0.1:%s:%d", host, ep.ContainerPort))
		case host != "":
			bindings = append(bindings, fmt.Sprintf("%s:%d", host, ep.ContainerPort))
		default:
			bindings = append(bindings, strconv.Itoa(ep.ContainerPort))
		}
	}

	return bindings
}
