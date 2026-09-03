package docker

import "fmt"

// Compose service names.
const (
	ServiceWeb         = "web"
	ServiceDatabase    = "database"
	ServiceAdminer     = "adminer"
	ServiceMailer      = "mailer"
	ServiceWorker      = "worker"
	ServiceScheduler   = "scheduler"
	ServiceQueue       = "queue"
	ServiceSearch      = "search"
	ServiceCache       = "cache"
	ServiceStorage     = "storage"
	ServiceStorageInit = "storage-init"
	ServiceBlackfire   = "blackfire"
	ServiceTideways    = "tideways-daemon"
)

// Endpoint names, the keys under docker.services.<service>.ports.
const (
	PortHTTP                    = "http"
	PortHTTPAlt                 = "http_alt"
	PortAdminWatcher            = "admin_watcher"
	PortAdminWatcherHMR         = "admin_watcher_hmr"
	PortStorefrontWatcher       = "storefront_watcher"
	PortStorefrontWatcherAssets = "storefront_watcher_assets"
	PortMySQL                   = "mysql"
	PortSMTP                    = "smtp"
	PortAMQP                    = "amqp"
	PortManagement              = "management"
	PortS3                      = "s3"
	PortConsole                 = "console"
)

// Variant names, the values of docker.services.<service>.type.
const (
	DatabaseMariaDB = "mariadb"
	DatabaseMySQL   = "mysql"
	QueueLavinMQ    = "lavinmq"
	QueueRabbitMQ   = "rabbitmq"
)

// Subdomains of the web service's proxy routes. They are constants (not
// endpoint subdomains) because web's routes are custom: the admin watch port
// is dynamic and the storefront watcher shares one hostname across three
// routes, neither of which a plain endpoint can model.
const (
	subdomainAdminWatch      = "admin-watch"
	subdomainStorefrontWatch = "storefront-watch"
)

// role classifies what a service endpoint is for.
type role int

const (
	// roleInternal endpoints serve other containers or host tooling; they are
	// never shown as a service URL.
	roleInternal role = iota
	// roleUI is the service's user-facing URL, shown in the dev dashboard's
	// Access table and in `project proxy list`.
	roleUI
	// roleAPI is a machine-consumed URL, e.g. the S3 public URL baked into the
	// shop's environment.
	roleAPI
)

// endpoint describes one addressable port of a compose service: the fixed
// container-side port, the ports key controlling the host port in plain mode,
// and the proxy subdomain it is routed at in proxy mode.
type endpoint struct {
	// Name is the key under docker.services.<service>.ports. Empty means the
	// generator never publishes the port on the host (compose network only).
	Name string
	// Label names the port in port-conflict messages.
	Label string
	Role  role
	// ContainerPort is the fixed container-side port.
	ContainerPort int
	// DefaultHostPort is the host port used in plain mode when no override is
	// configured. Zero publishes on a random host port until an override pins
	// it.
	DefaultHostPort int
	// Loopback binds the published port to 127.0.0.1 instead of all
	// interfaces.
	Loopback bool
	// Subdomain is the proxy route subdomain; empty means the endpoint is not
	// routed through the proxy.
	Subdomain string
}

// published reports whether the endpoint is published on the host in plain
// mode: it has a ports key and is not disabled.
func (ep endpoint) published(ports Ports) bool {
	if ep.Name == "" {
		return false
	}
	port, ok := ports[ep.Name]
	return !ok || !port.Disabled()
}

// hostPort returns the fixed host port the endpoint is published on in plain
// mode: the configured override when set, otherwise the catalog default. It
// returns 0 when the endpoint is not published, disabled, or published on a
// random port.
func (ep endpoint) hostPort(ports Ports) int {
	if !ep.published(ports) {
		return 0
	}

	if port, ok := ports[ep.Name]; ok && port > 0 {
		return int(port)
	}

	return ep.DefaultHostPort
}

// url resolves where the endpoint is reachable from the host given the run
// mode: proxied (proxyHost != "") or plain host-port mode honoring port
// overrides. It returns "" when the endpoint has no fixed host address (plain
// mode with the port disabled, random or never published; proxy mode without
// a route).
func (ep endpoint) url(proxyHost string, ports Ports) string {
	if proxyHost != "" {
		if ep.Subdomain == "" {
			return ""
		}
		return subdomainURL(ep.Subdomain, proxyHost)
	}

	if port := ep.hostPort(ports); port > 0 {
		return loopbackURL(port)
	}

	return ""
}

// variant is one implementation of a service the user can select via
// docker.services.<service>.type.
type variant struct {
	// Name is the type value.
	Name string
	// Image is the image repository; DefaultTag is used when no version is
	// configured.
	Image      string
	DefaultTag string
}

// service is the complete definition of one compose service the dev
// environment can contain: its display metadata, when it is generated, its
// selectable implementations, its endpoints, and how its compose spec is
// built from the environment. The catalog is the single source of truth for
// the config schema, compose generation, port publishing, proxy routing,
// port-conflict detection, URL generation and container discovery.
type service struct {
	// Name is the compose service name.
	Name string
	// Label is the human-readable name shown in the dashboard and proxy list.
	Label string
	// Username and Password are the credentials shown in the Access table;
	// empty means the service is open without auth.
	Username string
	Password string
	// Hidden services never appear in the Access table (web, database, cache,
	// storage-init).
	Hidden bool
	// Background marks a long-running console process without endpoints; the
	// dashboard lists it under "Background processing" instead of Access.
	Background bool
	// requires gates the service on the environment; nil means always.
	requires func(*Environment) bool
	// Variants lists the selectable implementations; the first is the
	// default. Empty means the service has a single fixed implementation and
	// accepts neither type nor version.
	Variants  []variant
	Endpoints []endpoint
	// build produces the service's compose spec for the environment. It must
	// not set ports, routing labels or proxy networks for routed endpoints;
	// the generator derives those from Endpoints.
	build func(e *Environment, svc service) composeService
}

// storageS3 is the S3 API endpoint. It is a package variable (not only a
// catalog entry) because the web service's build function needs it for the
// public media URL and must not reach back into the catalog during package
// initialization.
var storageS3 = endpoint{Name: PortS3, Label: "S3 API", Role: roleAPI, ContainerPort: 9000, DefaultHostPort: 9000, Subdomain: "s3"}

// catalog lists every compose service the dev environment can contain, in
// the order the services appear in the generated file.
var catalog = []service{
	{
		Name:   ServiceWeb,
		Label:  "Shop",
		Hidden: true,
		Endpoints: []endpoint{
			{Name: PortHTTP, Label: "Shop (Caddy)", Role: roleUI, ContainerPort: 8000, DefaultHostPort: 8000},
			{Name: PortHTTPAlt, Label: "Shop (alternative HTTP)", ContainerPort: 8080, DefaultHostPort: 8080},
			{Name: PortStorefrontWatcherAssets, Label: "Storefront watcher assets", ContainerPort: 9999, DefaultHostPort: 9999},
			{Name: PortStorefrontWatcher, Label: "Storefront watcher", ContainerPort: 9998, DefaultHostPort: 9998},
			{Name: PortAdminWatcher, Label: "Admin watcher (Vite)", ContainerPort: 5173, DefaultHostPort: 5173},
			{Name: PortAdminWatcherHMR, Label: "Admin watcher HMR", ContainerPort: 5773, DefaultHostPort: 5773},
		},
		build: buildWeb,
	},
	{
		Name:   ServiceDatabase,
		Label:  "Database",
		Hidden: true,
		Variants: []variant{
			{Name: DatabaseMariaDB, Image: "mariadb", DefaultTag: "11.8"},
			{Name: DatabaseMySQL, Image: "mysql", DefaultTag: "8.4"},
		},
		// Published on a random loopback port by default so host-side tools
		// (e.g. `project sql`) reach it without conflicts; a configured port
		// pins it for IDE connections.
		Endpoints: []endpoint{{Name: PortMySQL, Label: "Database (MySQL protocol)", Role: roleInternal, ContainerPort: 3306, Loopback: true}},
		build:     buildDatabase,
	},
	{
		Name:     ServiceAdminer,
		Label:    "Adminer",
		Username: "root",
		Password: "root",
		Endpoints: []endpoint{
			{Name: PortHTTP, Label: "Adminer", Role: roleUI, ContainerPort: 8080, DefaultHostPort: 9080, Subdomain: "adminer"},
		},
		build: buildAdminer,
	},
	{
		Name:  ServiceMailer,
		Label: "Mailpit",
		Endpoints: []endpoint{
			{Name: PortSMTP, Label: "Mailpit SMTP", ContainerPort: 1025, DefaultHostPort: 1025},
			{Name: PortHTTP, Label: "Mailpit UI", Role: roleUI, ContainerPort: 8025, DefaultHostPort: 8025, Subdomain: "mailer"},
		},
		build: buildMailer,
	},
	{
		Name:       ServiceWorker,
		Label:      "Queue worker",
		Hidden:     true,
		Background: true,
		requires:   dedicatedWorker,
		build:      buildConsole("messenger:consume", "--all"),
	},
	{
		Name:       ServiceScheduler,
		Label:      "Scheduled tasks",
		Hidden:     true,
		Background: true,
		requires:   dedicatedWorker,
		build:      buildConsole("scheduled-task:run"),
	},
	{
		Name:     ServiceQueue,
		Label:    "Queue",
		Username: "guest",
		Password: "guest",
		requires: requiresAMQP,
		Variants: []variant{
			{Name: QueueLavinMQ, Image: "cloudamqp/lavinmq", DefaultTag: "latest"},
			{Name: QueueRabbitMQ, Image: "rabbitmq", DefaultTag: "4-management"},
		},
		Endpoints: []endpoint{
			{Name: PortManagement, Label: "Queue management UI", Role: roleUI, ContainerPort: 15672, DefaultHostPort: 15672, Loopback: true, Subdomain: "queue"},
			{Name: PortAMQP, Label: "AMQP", ContainerPort: 5672, DefaultHostPort: 5672, Loopback: true},
		},
		build: buildQueue,
	},
	{
		Name:     ServiceSearch,
		Label:    "Search",
		requires: requiresElasticsearch,
		Endpoints: []endpoint{
			{Name: PortHTTP, Label: "OpenSearch", Role: roleUI, ContainerPort: 9200, DefaultHostPort: 9200, Loopback: true, Subdomain: "search"},
		},
		build: buildSearch,
	},
	{
		Name:      ServiceCache,
		Label:     "Cache",
		Hidden:    true,
		requires:  requiresRedis,
		Endpoints: []endpoint{{Label: "Redis", Role: roleInternal, ContainerPort: 6379}},
		build:     buildCache,
	},
	{
		Name:     ServiceStorage,
		Label:    "Storage (S3)",
		Username: "shopware",
		Password: "shopware",
		requires: requiresS3,
		Endpoints: []endpoint{
			storageS3,
			{Name: PortConsole, Label: "Storage console", Role: roleUI, ContainerPort: 9001, DefaultHostPort: 9001, Subdomain: "storage"},
		},
		build: buildStorage,
	},
	{
		Name:     ServiceStorageInit,
		Label:    "Storage bucket init",
		Hidden:   true,
		requires: requiresS3,
		build:    buildStorageInit,
	},
	{
		Name:     ServiceBlackfire,
		Label:    "Blackfire agent",
		Hidden:   true,
		requires: (*Environment).blackfireConfigured,
		build:    buildBlackfire,
	},
	{
		Name:     ServiceTideways,
		Label:    "Tideways daemon",
		Hidden:   true,
		requires: (*Environment).tidewaysConfigured,
		build:    buildTideways,
	},
}

func requiresAMQP(e *Environment) bool          { return e.features.AMQP }
func requiresElasticsearch(e *Environment) bool { return e.features.Elasticsearch }
func requiresRedis(e *Environment) bool         { return e.features.needsRedis() }
func requiresS3(e *Environment) bool            { return e.features.S3 }
func dedicatedWorker(e *Environment) bool       { return e.dedicatedWorker }

// activeServices returns the catalog entries generated for the environment,
// in catalog order.
func (e *Environment) activeServices() []service {
	services := make([]service, 0, len(catalog))
	for _, svc := range catalog {
		if svc.requires == nil || svc.requires(e) {
			services = append(services, svc)
		}
	}

	return services
}

// byName returns the catalog entry for a compose service name, or nil for an
// unknown service.
func byName(name string) *service {
	for i := range catalog {
		if catalog[i].Name == name {
			return &catalog[i]
		}
	}

	return nil
}

// mustEndpoint returns the named endpoint of the named service; it panics on
// an unknown pair and is meant for catalog constants.
func mustEndpoint(serviceName, endpointName string) endpoint {
	if svc := byName(serviceName); svc != nil {
		if ep := svc.endpointNamed(endpointName); ep != nil {
			return *ep
		}
	}

	panic(fmt.Sprintf("docker: unknown endpoint %s.%s", serviceName, endpointName))
}

// configurableServices returns the services that accept settings under
// docker.services: those with a publishable endpoint or selectable variants.
func configurableServices() []service {
	var services []service
	for _, svc := range catalog {
		if svc.configurable() {
			services = append(services, svc)
		}
	}

	return services
}

// configurable reports whether the service accepts settings under
// docker.services.
func (s service) configurable() bool {
	return len(s.Variants) > 0 || len(s.publishedEndpoints()) > 0
}

// endpointNamed returns the endpoint with the given ports key, or nil.
func (s service) endpointNamed(name string) *endpoint {
	if name == "" {
		return nil
	}
	for i := range s.Endpoints {
		if s.Endpoints[i].Name == name {
			return &s.Endpoints[i]
		}
	}

	return nil
}

// publishedEndpoints returns the endpoints that have a ports key, in catalog
// order.
func (s service) publishedEndpoints() []endpoint {
	var eps []endpoint
	for _, ep := range s.Endpoints {
		if ep.Name != "" {
			eps = append(eps, ep)
		}
	}

	return eps
}

// uiEndpoint returns the endpoint shown as the service's URL, or nil when the
// service has no user-facing endpoint.
func (s service) uiEndpoint() *endpoint {
	for i := range s.Endpoints {
		if s.Endpoints[i].Role == roleUI {
			return &s.Endpoints[i]
		}
	}

	return nil
}

// variantNamed returns the implementation selected by name, the default for
// an empty name, or nil when the service has no such variant.
func (s service) variantNamed(name string) *variant {
	if len(s.Variants) == 0 {
		return nil
	}
	if name == "" {
		return &s.Variants[0]
	}
	for i := range s.Variants {
		if s.Variants[i].Name == name {
			return &s.Variants[i]
		}
	}

	return nil
}

// variantNames returns the selectable type values, default first.
func (s service) variantNames() []string {
	names := make([]string, 0, len(s.Variants))
	for _, v := range s.Variants {
		names = append(names, v.Name)
	}

	return names
}

// selected resolves the implementation and image tag chosen for the service
// via docker.services.<service>.type and .version, falling back to the catalog
// defaults. Unknown types are rejected at config read time, so a miss here
// only happens for hand-built environments and takes the default.
func (s service) selected(e *Environment) (variant, string) {
	typeName, version := "", ""
	if cfg := e.setting(s.Name); cfg != nil {
		typeName, version = cfg.Type, cfg.Version
	}

	v := s.variantNamed(typeName)
	if v == nil {
		v = s.variantNamed("")
	}
	if version == "" {
		version = v.DefaultTag
	}

	return *v, version
}

// proxyURL returns the service's UI endpoint URL through the shared proxy, or
// "" when the service has no routed UI endpoint.
func (s service) proxyURL(hostname string) string {
	ep := s.uiEndpoint()
	if ep == nil || ep.Subdomain == "" {
		return ""
	}

	return subdomainURL(ep.Subdomain, hostname)
}

// publishedURL resolves the service's UI endpoint URL from the actually
// published ports (container→host, e.g. from `docker compose ps`) or, when
// the project is proxied and nothing is published, from the endpoint's proxy
// subdomain. It returns "" when the service is not reachable from the host.
func (s service) publishedURL(published map[int]int, proxyHost string) string {
	ep := s.uiEndpoint()
	if ep == nil {
		return ""
	}

	if port, ok := published[ep.ContainerPort]; ok && port != 0 {
		return loopbackURL(port)
	}

	if proxyHost != "" {
		return s.proxyURL(proxyHost)
	}

	return ""
}

// loopbackURL is the plain-mode URL for a port published on the host.
func loopbackURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// subdomainURL is the proxy-mode URL for a routed subdomain.
func subdomainURL(subdomain, hostname string) string {
	return fmt.Sprintf("https://%s.%s", subdomain, hostname)
}

// Link is a labelled URL of a service's user interface.
type Link struct {
	Label string
	URL   string
}

// ServiceLink returns the label and proxy URL of a compose service's user
// interface, for listing a proxied project's services. It reports false for
// unknown, hidden and unrouted services.
func ServiceLink(service, hostname string) (Link, bool) {
	def := byName(service)
	if def == nil || def.Hidden {
		return Link{}, false
	}

	url := def.proxyURL(hostname)
	if url == "" {
		return Link{}, false
	}

	return Link{Label: def.Label, URL: url}, true
}

// DefaultShopURL is the shop URL with default ports and no proxy.
var DefaultShopURL = loopbackURL(mustEndpoint(ServiceWeb, PortHTTP).DefaultHostPort)
