package docker

import (
	"fmt"
	"path/filepath"

	"github.com/shyim/go-composer"

	"github.com/shopware/shopware-cli/internal/symfony"
	"github.com/shopware/shopware-cli/internal/system"
)

const nodeVersion = "24"

// Profiler names for the PHP profiler setting.
const (
	ProfilerBlackfire = "blackfire"
	ProfilerTideways  = "tideways"
	ProfilerXdebug    = "xdebug"
	ProfilerPcov      = "pcov"
	ProfilerSpx       = "spx"
)

// Profilers is the ordered list of profiler names. The empty string means "no
// profiler".
var Profilers = []string{"", ProfilerXdebug, ProfilerBlackfire, ProfilerTideways, ProfilerPcov, ProfilerSpx}

// ProfilerIsPaid reports whether the given profiler is a commercial product
// that requires a paid account or plan. Blackfire and Tideways are paid SaaS
// products; xdebug, pcov and spx are free and open source.
func ProfilerIsPaid(profiler string) bool {
	return profiler == ProfilerBlackfire || profiler == ProfilerTideways
}

// Proxy carries the data needed to route a project's services through the
// shared shopware-cli proxy by hostname instead of fixed host ports.
type Proxy struct {
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

// PHP configures the docker-dev image the PHP containers run.
type PHP struct {
	// Version is the PHP version (e.g. "8.3"); empty means the default.
	Version string
	// Profiler is one of the Profiler* names; empty means none.
	Profiler             string
	BlackfireServerID    string
	BlackfireServerToken string
	TidewaysAPIKey       string
}

// version returns the configured PHP version or the default.
func (p PHP) version() string {
	if p.Version != "" {
		return p.Version
	}
	return "8.3"
}

// WebImage returns the docker-dev image the web and console services run,
// derived from the PHP version and the pinned Node version.
func (p PHP) WebImage() string {
	return fmt.Sprintf("ghcr.io/shopware/docker-dev:php%s-node%s-caddy", p.version(), nodeVersion)
}

// ServiceSettings configures one compose service of the dev environment. It
// is the YAML shape of docker.services.<service>.
type ServiceSettings struct {
	// Implementation to run, for services that offer more than one (database: mariadb or mysql, queue: lavinmq or rabbitmq).
	Type string `yaml:"type,omitempty"`
	// Image version (tag) to run. Defaults per implementation.
	Version string `yaml:"version,omitempty"`
	// Host ports the service's endpoints are published on, keyed by endpoint name. false disables publishing an endpoint.
	Ports Ports `yaml:"ports,omitempty"`
}

// Settings holds the per-service settings keyed by compose service name.
type Settings map[string]*ServiceSettings

// Ports returns the host-port overrides of a service, nil when none are
// configured. Nil-safe.
func (s Settings) Ports(name string) Ports {
	if svc := s[name]; svc != nil {
		return svc.Ports
	}
	return nil
}

// Options are the caller-provided inputs of a dev environment: how PHP is set
// up, what the user configured per service, and whether the project is served
// through the shared proxy.
type Options struct {
	// PHP configures the docker-dev image and its profiler.
	PHP PHP
	// Services are the per-service settings from docker.services.
	Services Settings
	// Proxy, when set, serves the project through the shared proxy: routed
	// services publish no host ports and instead join the proxy network with
	// Traefik routing labels. Nil keeps fixed-port mode.
	Proxy *Proxy
	// User is the "uid:gid" the PHP containers run as so that writes to the
	// bind-mounted project are owned by the host user. Empty resolves the
	// host user of the project folder.
	User string
}

// Environment is a project's Docker dev environment: everything needed to
// generate its compose file, probe its host ports, resolve its URLs and
// inspect its containers. It is a snapshot of the project (composer.lock,
// Symfony configuration) and the options it was built with; rebuild it after
// the configuration changes.
type Environment struct {
	root string
	// features are the composer.lock packages that add optional services.
	features features
	proxy    *Proxy
	php      PHP
	user     string
	// dedicatedWorker adds the long-running console processes (messenger
	// worker, scheduled tasks). It is needed when Shopware's admin worker is
	// disabled, because the queue is then no longer dispatched from the
	// browser.
	dedicatedWorker bool
	settings        Settings
}

// NewEnvironment resolves the dev environment of the project at projectRoot:
// it reads composer.lock for the optional services, the Symfony configuration
// for the admin worker toggle, and the project folder's owner as the container
// user unless opts.User is set. A project without a readable composer.lock has
// no environment.
func NewEnvironment(projectRoot string, opts Options) (*Environment, error) {
	lock, err := composer.ReadLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return nil, fmt.Errorf("failed to read composer.lock: %w", err)
	}

	// The compose file targets the dev environment, so evaluate the admin
	// worker toggle for dev. A dedicated worker is only needed when it is off.
	adminWorkerEnabled, err := symfony.IsAdminWorkerEnabledForProject(projectRoot, "dev")
	if err != nil {
		return nil, fmt.Errorf("failed to read admin worker config: %w", err)
	}

	user := opts.User
	if user == "" {
		user = system.ProjectUserSpec(projectRoot)
	}

	return &Environment{
		root:            projectRoot,
		features:        featuresFromLock(lock),
		proxy:           opts.Proxy,
		php:             opts.PHP,
		user:            user,
		dedicatedWorker: !adminWorkerEnabled,
		settings:        opts.Services,
	}, nil
}

// WebImage returns the docker-dev image the web and console services run.
func (e *Environment) WebImage() string {
	return e.php.WebImage()
}

// ProxyHost returns the project's proxy hostname, or "" in fixed-port mode.
func (e *Environment) ProxyHost() string {
	return e.proxyHost()
}

// AdminWatchURL returns where the administration watcher is reachable from
// the host: its proxy subdomain when the project is proxied, otherwise its
// published port honoring docker.services.web.ports. It returns "" when the
// port is disabled.
func (e *Environment) AdminWatchURL() string {
	return e.webWatchURL(subdomainAdminWatch, PortAdminWatcher)
}

// StorefrontWatchURL returns where the storefront watcher is reachable from
// the host, resolved like AdminWatchURL.
func (e *Environment) StorefrontWatchURL() string {
	return e.webWatchURL(subdomainStorefrontWatch, PortStorefrontWatcher)
}

func (e *Environment) webWatchURL(subdomain, endpointName string) string {
	if e.proxy != nil {
		return subdomainURL(subdomain, e.proxy.Hostname)
	}

	return mustEndpoint(ServiceWeb, endpointName).url("", e.ports(ServiceWeb))
}

func (e *Environment) proxyHost() string {
	if e.proxy == nil {
		return ""
	}
	return e.proxy.Hostname
}

// ports returns the host-port overrides of a service, nil when none are
// configured.
func (e *Environment) ports(service string) Ports {
	return e.settings.Ports(service)
}

// setting returns the settings of a service, nil when none are configured.
func (e *Environment) setting(service string) *ServiceSettings {
	return e.settings[service]
}

// blackfireConfigured reports whether the Blackfire profiler is selected
// with credentials, which is what its agent container needs.
func (e *Environment) blackfireConfigured() bool {
	return e.php.Profiler == ProfilerBlackfire && e.php.BlackfireServerID != "" && e.php.BlackfireServerToken != ""
}

// tidewaysConfigured reports whether the Tideways profiler is selected with
// an API key, which is what its daemon container needs.
func (e *Environment) tidewaysConfigured() bool {
	return e.php.Profiler == ProfilerTideways && e.php.TidewaysAPIKey != ""
}
