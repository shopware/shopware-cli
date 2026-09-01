package docker

import (
	"fmt"

	"github.com/shopware/shopware-cli/internal/shop"
)

// Subdomains of the web service's proxy routes. They are constants (not
// endpoint subdomains) because web's routes are built by webProxyRoutes: the
// admin watch port is dynamic and the storefront watcher shares one hostname
// across three routes, neither of which a plain endpoint can model.
const (
	SubdomainAdminWatch      = "admin-watch"
	SubdomainStorefrontWatch = "storefront-watch"
)

// EndpointRole classifies what a service endpoint is for.
type EndpointRole int

const (
	// RoleInternal endpoints serve other containers or host tooling; they are
	// never shown as a service URL.
	RoleInternal EndpointRole = iota
	// RoleUI is the service's user-facing URL, shown in the dev dashboard's
	// Access table and in `project proxy list`.
	RoleUI
	// RoleAPI is a machine-consumed URL, e.g. the S3 public URL baked into the
	// shop's environment.
	RoleAPI
)

// Endpoint describes one addressable port of a compose service: the fixed
// container-side port, the docker.ports key controlling the host port in plain
// mode, and the proxy subdomain it is routed at in proxy mode.
type Endpoint struct {
	// Key is the docker.ports config key (shop.DockerPort*). Empty means the
	// port is never published on the host (compose network only).
	Key string
	// Label names the port in port-conflict messages.
	Label string
	Role  EndpointRole
	// ContainerPort is the fixed container-side port.
	ContainerPort int
	// DefaultHostPort is the host port used in plain mode when no override is
	// configured.
	DefaultHostPort int
	// Subdomain is the proxy route subdomain; empty means the endpoint is not
	// routed through the proxy.
	Subdomain string
}

// ServiceDefinition describes one compose service the dev environment can
// contain: its display metadata, feature gate, and endpoints. It is the single
// source of truth for port publishing, proxy routing, port-conflict detection
// and URL generation — the compose service spec (image, env, healthcheck)
// stays hand-written in compose.go.
type ServiceDefinition struct {
	// Name is the compose service name.
	Name string
	// Label is the human-readable name shown in the dashboard and proxy list.
	Label string
	// Username and Password are the credentials shown in the Access table;
	// empty means the service is open without auth.
	Username string
	Password string
	// Hidden services never appear in the Access table (web, database, redis,
	// rustfs-init).
	Hidden bool
	// Feature gates the service on the composer.lock contents; nil means the
	// service is always generated.
	Feature func(LockFeatures) bool
	// routeSubdomains, when non-nil, overrides the endpoint-derived proxy
	// subdomains (web only — see the subdomain constants above). An empty
	// entry denotes the project's root hostname.
	routeSubdomains []string
	Endpoints       []Endpoint
}

func requiresAMQP(f LockFeatures) bool          { return f.AMQP }
func requiresElasticsearch(f LockFeatures) bool { return f.Elasticsearch }
func requiresS3(f LockFeatures) bool            { return f.S3 }
func requiresRedis(f LockFeatures) bool         { return f.NeedsRedis() }

// Services is the catalog of every compose service the dev environment can
// contain, in the order the services appear in the generated file.
var Services = []ServiceDefinition{
	{
		Name:            "web",
		Label:           "Shop",
		Hidden:          true,
		routeSubdomains: []string{"", SubdomainAdminWatch, SubdomainStorefrontWatch},
		Endpoints: []Endpoint{
			{Key: shop.DockerPortWeb, Label: "Shop (Caddy)", Role: RoleUI, ContainerPort: 8000, DefaultHostPort: 8000},
			{Key: shop.DockerPortWebAlt, Label: "Shop (alternative HTTP)", ContainerPort: 8080, DefaultHostPort: 8080},
			{Key: shop.DockerPortStorefrontWatcherAssets, Label: "Storefront watcher assets", ContainerPort: 9999, DefaultHostPort: 9999},
			{Key: shop.DockerPortStorefrontWatcher, Label: "Storefront watcher", ContainerPort: 9998, DefaultHostPort: 9998},
			{Key: shop.DockerPortAdminWatcher, Label: "Admin watcher (Vite)", ContainerPort: 5173, DefaultHostPort: 5173},
			{Key: shop.DockerPortAdminWatcherHMR, Label: "Admin watcher HMR", ContainerPort: 5773, DefaultHostPort: 5773},
		},
	},
	{
		Name:   "database",
		Label:  "Database",
		Hidden: true,
		// The host publish is a random loopback port hand-written in
		// compose.go; the endpoint only records the container port.
		Endpoints: []Endpoint{{Label: "MariaDB", ContainerPort: 3306}},
	},
	{
		Name:     "adminer",
		Label:    "Adminer",
		Username: "root",
		Password: "root",
		Endpoints: []Endpoint{
			{Key: shop.DockerPortAdminer, Label: "Adminer", Role: RoleUI, ContainerPort: 8080, DefaultHostPort: 9080, Subdomain: "adminer"},
		},
	},
	{
		Name:  "mailer",
		Label: "Mailpit",
		Endpoints: []Endpoint{
			{Key: shop.DockerPortMailerSMTP, Label: "Mailpit SMTP", ContainerPort: 1025, DefaultHostPort: 1025},
			{Key: shop.DockerPortMailerWeb, Label: "Mailpit UI", Role: RoleUI, ContainerPort: 8025, DefaultHostPort: 8025, Subdomain: "mailer"},
		},
	},
	{
		Name:     "lavinmq",
		Label:    "Queue",
		Username: "guest",
		Password: "guest",
		Feature:  requiresAMQP,
		Endpoints: []Endpoint{
			{Key: shop.DockerPortAMQPManagement, Label: "LavinMQ management", Role: RoleUI, ContainerPort: 15672, DefaultHostPort: 15672, Subdomain: "lavinmq"},
			{Key: shop.DockerPortAMQP, Label: "AMQP", ContainerPort: 5672, DefaultHostPort: 5672},
		},
	},
	{
		Name:    "opensearch",
		Label:   "Search",
		Feature: requiresElasticsearch,
		Endpoints: []Endpoint{
			{Key: shop.DockerPortElasticsearch, Label: "OpenSearch", Role: RoleUI, ContainerPort: 9200, DefaultHostPort: 9200, Subdomain: "opensearch"},
		},
	},
	{
		Name:      "redis",
		Label:     "Redis",
		Hidden:    true,
		Feature:   requiresRedis,
		Endpoints: []Endpoint{{Label: "Redis", ContainerPort: 6379}},
	},
	{
		Name:     "rustfs",
		Label:    "S3 (RustFS)",
		Username: "shopware",
		Password: "shopware",
		Feature:  requiresS3,
		Endpoints: []Endpoint{
			{Key: shop.DockerPortS3, Label: "S3 API (RustFS)", Role: RoleAPI, ContainerPort: 9000, DefaultHostPort: 9000, Subdomain: "s3"},
			{Key: shop.DockerPortS3Console, Label: "RustFS console", Role: RoleUI, ContainerPort: 9001, DefaultHostPort: 9001, Subdomain: "rustfs"},
		},
	},
	{
		Name:    "rustfs-init",
		Label:   "RustFS bucket init",
		Hidden:  true,
		Feature: requiresS3,
	},
}

// ActiveServices returns the catalog entries generated for the given lock
// features.
func ActiveServices(features LockFeatures) []ServiceDefinition {
	services := make([]ServiceDefinition, 0, len(Services))
	for _, svc := range Services {
		if svc.Feature != nil && !svc.Feature(features) {
			continue
		}
		services = append(services, svc)
	}

	return services
}

// ServiceByName returns the catalog entry for a compose service name, or nil
// for a service the generator never emits (e.g. user compose overrides).
func ServiceByName(name string) *ServiceDefinition {
	for i := range Services {
		if Services[i].Name == name {
			return &Services[i]
		}
	}

	return nil
}

// EndpointByKey returns the catalog endpoint for a docker.ports config key, or
// nil for an unknown key.
func EndpointByKey(key string) *Endpoint {
	for si := range Services {
		for ei := range Services[si].Endpoints {
			if Services[si].Endpoints[ei].Key == key {
				return &Services[si].Endpoints[ei]
			}
		}
	}

	return nil
}

// ShopEndpoint returns the web service's shop endpoint.
func ShopEndpoint() Endpoint {
	return *EndpointByKey(shop.DockerPortWeb)
}

// RoutedSubdomains returns every proxy subdomain the active services are
// routed at, in catalog order. The web service contributes the root hostname
// ("") plus the watcher subdomains.
func RoutedSubdomains(features LockFeatures) []string {
	var subs []string
	for _, svc := range ActiveServices(features) {
		subs = append(subs, svc.RoutedSubdomains()...)
	}

	return subs
}

// RoutedSubdomains returns the proxy subdomains this service is routed at;
// an empty entry denotes the project's root hostname.
func (d ServiceDefinition) RoutedSubdomains() []string {
	if d.routeSubdomains != nil {
		return d.routeSubdomains
	}

	var subs []string
	for _, ep := range d.Endpoints {
		if ep.Subdomain != "" {
			subs = append(subs, ep.Subdomain)
		}
	}

	return subs
}

// UIEndpoint returns the endpoint shown as the service's URL, or nil when the
// service has no user-facing endpoint.
func (d ServiceDefinition) UIEndpoint() *Endpoint {
	for i := range d.Endpoints {
		if d.Endpoints[i].Role == RoleUI {
			return &d.Endpoints[i]
		}
	}

	return nil
}

// portKeys returns the docker.ports keys of the service's endpoints, in
// endpoint order.
func (d ServiceDefinition) portKeys() []string {
	keys := make([]string, 0, len(d.Endpoints))
	for _, ep := range d.Endpoints {
		if ep.Key != "" {
			keys = append(keys, ep.Key)
		}
	}

	return keys
}

// proxyRoutes returns one proxy route per routed endpoint.
func (d ServiceDefinition) proxyRoutes() []proxyRoute {
	var routes []proxyRoute
	for _, ep := range d.Endpoints {
		if ep.Subdomain != "" {
			routes = append(routes, proxyRoute{subdomain: ep.Subdomain, containerPort: ep.ContainerPort})
		}
	}

	return routes
}

// loopbackHTTPURL is the plain-mode URL for a port published on the host.
func loopbackHTTPURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// proxySubdomainURL is the proxy-mode URL for a routed subdomain.
func proxySubdomainURL(subdomain, hostname string) string {
	return fmt.Sprintf("https://%s.%s", subdomain, hostname)
}

// ProxyURL returns the service's UI endpoint URL through the shared proxy, or
// "" when the service has no routed UI endpoint.
func (d ServiceDefinition) ProxyURL(hostname string) string {
	ep := d.UIEndpoint()
	if ep == nil || ep.Subdomain == "" {
		return ""
	}

	return proxySubdomainURL(ep.Subdomain, hostname)
}

// AccessURL resolves the service's UI endpoint URL from the actual published
// ports (container→host, e.g. from `docker compose ps`) or, when the project
// is proxied and nothing is published, from the endpoint's proxy subdomain. It
// returns "" when the service is not reachable from the host.
func (d ServiceDefinition) AccessURL(published map[int]int, proxyHost string) string {
	ep := d.UIEndpoint()
	if ep == nil {
		return ""
	}

	if port, ok := published[ep.ContainerPort]; ok && port != 0 {
		return loopbackHTTPURL(port)
	}

	if proxyHost != "" {
		return d.ProxyURL(proxyHost)
	}

	return ""
}

// EndpointURL resolves where an endpoint is reachable from the host, given the
// run mode: proxied (proxyHost != "") or plain host-port mode honoring
// docker.ports overrides. It returns "" when the endpoint is not reachable
// (plain mode with the port disabled or never published, proxy mode without a
// route).
func EndpointURL(ep Endpoint, proxyHost string, ports shop.ConfigDockerPorts) string {
	if proxyHost != "" {
		if ep.Subdomain == "" {
			return ""
		}
		return proxySubdomainURL(ep.Subdomain, proxyHost)
	}

	if ep.Key == "" {
		return ""
	}
	if port := HostPort(ports, ep.Key); port > 0 {
		return loopbackHTTPURL(port)
	}

	return ""
}
