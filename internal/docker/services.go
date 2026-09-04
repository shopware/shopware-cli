package docker

import (
	"fmt"
	"slices"
	"strconv"
)

// customRoutes holds the proxy routes of services whose routing cannot be
// derived from their endpoints. Only web needs it: the admin watch port is
// dynamic and the storefront watcher shares one hostname across three routes.
var customRoutes = map[string]func(*Proxy) []proxyRoute{
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

	return serviceRoutes(svc, e.proxy)
}

func serviceRoutes(svc service, p *Proxy) []proxyRoute {
	if custom, ok := customRoutes[svc.Name]; ok {
		return custom(p)
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
		subs = append(subs, serviceSubdomains(svc)...)
	}

	return subs
}

// serviceSubdomains returns the distinct proxy subdomains the service is
// routed at, in route order; an empty entry denotes the project's root
// hostname.
func serviceSubdomains(svc service) []string {
	var subs []string
	for _, route := range serviceRoutes(svc, &Proxy{}) {
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
