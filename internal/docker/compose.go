package docker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shyim/go-composer"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/symfony"
	"github.com/shopware/shopware-cli/internal/system"
)

const nodeVersion = "24"

// backgroundProcessLimits bound the dedicated console processes so they recycle
// periodically (paired with restart: unless-stopped in the compose file).
var backgroundProcessLimits = []string{"--time-limit=300", "--memory-limit=512M"}

// BackgroundService describes a long-running console process added to the dev
// compose file when Shopware's admin worker is disabled. It is the single source
// of truth for both compose generation (name + command) and the dev TUI overview
// (name + Label), so the two never drift apart.
type BackgroundService struct {
	// Name is the compose service name.
	Name string
	// Label is the human-readable name shown in the dev TUI overview.
	Label   string
	command []string
}

// BackgroundServices is the ordered set of dedicated processes generated when
// the admin worker is disabled: the message queue consumer and the scheduled
// task runner.
var BackgroundServices = []BackgroundService{
	{Name: "worker", Label: "Queue worker", command: append([]string{"messenger:consume", "--all"}, backgroundProcessLimits...)},
	{Name: "scheduler", Label: "Scheduled tasks", command: append([]string{"scheduled-task:run"}, backgroundProcessLimits...)},
}

// Profiler constants for the Docker dev environment.
const (
	ProfilerBlackfire = "blackfire"
	ProfilerTideways  = "tideways"
	ProfilerXdebug    = "xdebug"
	ProfilerPcov      = "pcov"
	ProfilerSpx       = "spx"
)

// Profilers is the ordered list of profiler names for the Docker dev environment.
// The empty string means "no profiler".
var Profilers = []string{"", ProfilerXdebug, ProfilerBlackfire, ProfilerTideways, ProfilerPcov, ProfilerSpx}

// ProfilerNeedsCredentials reports whether the given profiler requires API credentials.
func ProfilerNeedsCredentials(profiler string) bool {
	return profiler == ProfilerBlackfire || profiler == ProfilerTideways
}

// ProfilerIsPaid reports whether the given profiler is a commercial product
// that requires a paid account or plan. Blackfire and Tideways are paid SaaS
// products; xdebug, pcov and spx are free and open source.
func ProfilerIsPaid(profiler string) bool {
	return profiler == ProfilerBlackfire || profiler == ProfilerTideways
}

type ComposeOptions struct {
	PHPVersion           string
	PHPProfiler          string
	BlackfireServerID    string
	BlackfireServerToken string
	TidewaysAPIKey       string
	// User is the "uid:gid" the web container should run as so that
	// writes to the bind-mounted project (var/, files/, public/, ...)
	// are owned by the host user. Empty means: use the image default.
	User string
	// DedicatedWorker requests an extra service that runs
	// messenger:consume. It is needed when Shopware's admin worker is
	// disabled (shopware.admin_worker.enable_admin_worker: false), because
	// the message queue is then no longer dispatched from the browser.
	DedicatedWorker bool
	// Proxy, when set, generates the compose file in shared-proxy mode: the
	// routed services publish no host ports and instead join the proxy network
	// with Traefik routing labels. Nil (the default) keeps fixed-port mode.
	// Callers populate it via proxy.ComposeProxyOptions for proxy projects.
	Proxy *ProxyOptions
}

func (o *ComposeOptions) phpVersion() string {
	if o != nil && o.PHPVersion != "" {
		return o.PHPVersion
	}
	return "8.3"
}

// proxy returns the proxy options, or nil when not in proxy mode (also for a
// nil receiver), so buildCompose can branch on a single nil-safe accessor.
func (o *ComposeOptions) proxy() *ProxyOptions {
	if o == nil {
		return nil
	}
	return o.Proxy
}

// WebImage returns the docker-dev image the web and console services run,
// derived from the configured PHP version and the pinned Node version. Callers
// outside this package (the proxy CA-bundle builder) need the exact tag to
// operate on the same image the shop will run.
func WebImage(opts *ComposeOptions) string {
	return fmt.Sprintf("ghcr.io/shopware/docker-dev:php%s-node%s-caddy", opts.phpVersion(), nodeVersion)
}

func ComposeOptionsFromConfig(cfg *shop.Config) *ComposeOptions {
	if cfg == nil || cfg.Docker == nil {
		return nil
	}
	opts := &ComposeOptions{}
	if cfg.Docker.PHP != nil {
		opts.PHPVersion = cfg.Docker.PHP.Version
		opts.PHPProfiler = cfg.Docker.PHP.Profiler
		opts.BlackfireServerID = cfg.Docker.PHP.BlackfireServerID
		opts.BlackfireServerToken = cfg.Docker.PHP.BlackfireServerToken
		opts.TidewaysAPIKey = cfg.Docker.PHP.TidewaysAPIKey
	}
	return opts
}

// redisMessengerDSN is the Symfony Redis transport used when
// symfony/redis-messenger is in the lock (shopware/k8s-meta requires it). It
// points at the generated redis service (not localhost) so PHP containers can
// reach the queue on the compose network. If both Redis and AMQP messengers
// are present, this DSN wins over LavinMQ.
const redisMessengerDSN = "redis://redis:6379/messages/symfony/?auto_setup=true&serializer=1&stream_max_entries=0&dbindex=0"

const (
	rustfsAccessKey = "shopware"
	rustfsSecretKey = "shopware"
)

func GenerateComposeFile(lock *composer.Lock, opts *ComposeOptions) ([]byte, error) {
	doc := buildCompose(FeaturesFromLock(lock), opts)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}

	header := "# This file is managed by shopware-cli. Do not edit manually.\n" +
		"# Create a compose.override.yaml to customize services.\n" +
		"# See https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/\n\n"

	return append([]byte(header), out...), nil
}

func WriteComposeFile(projectFolder string, opts *ComposeOptions) error {
	if opts == nil {
		opts = &ComposeOptions{}
	}
	if opts.User == "" {
		opts.User = system.ProjectUserSpec(projectFolder)
	}

	// The compose file targets the dev environment, so evaluate the admin
	// worker toggle for dev. A dedicated worker is only needed when it is off.
	adminWorkerEnabled, err := symfony.IsAdminWorkerEnabledForProject(projectFolder, "dev")
	if err != nil {
		return fmt.Errorf("failed to read admin worker config: %w", err)
	}
	opts.DedicatedWorker = !adminWorkerEnabled

	lock, err := composer.ReadLock(filepath.Join(projectFolder, "composer.lock"))
	if err != nil {
		return fmt.Errorf("failed to read composer.lock: %w", err)
	}

	composeBytes, err := GenerateComposeFile(lock, opts)
	if err != nil {
		return fmt.Errorf("failed to generate compose.yaml: %w", err)
	}

	if err := ensureEnvLocalFile(projectFolder); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(projectFolder, "compose.yaml"), composeBytes, 0o644)
}

// ensureEnvLocalFile creates .env.local when it is missing. The generated
// compose file declares it as env_file for every PHP service, and Compose
// refuses to start when a declared env file does not exist — which is the
// state of every fresh clone, since .env.local is gitignored.
func ensureEnvLocalFile(projectFolder string) error {
	envLocalPath := filepath.Join(projectFolder, ".env.local")
	if _, err := os.Stat(envLocalPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	return os.WriteFile(envLocalPath, []byte(shop.EnvLocalDockerContent), 0o644)
}

func buildCompose(features LockFeatures, opts *ComposeOptions) composeFile {
	px := opts.proxy()

	webEnv := baseWebEnv(px)
	webEnv, webDependsOn := applyLockEnv(webEnv, px, features)
	webEnv = applyProfilerEnv(webEnv, opts)

	web := composeService{
		Image:       WebImage(opts),
		EnvFile:     []string{".env.local"},
		Environment: webEnv,
		DependsOn:   webDependsOn,
	}
	if opts != nil && opts.User != "" {
		web.User = opts.User
	}
	addVolumes(&web, px, ".:/var/www/html")
	publishOrRoute(&web, px, "web",
		[]string{"8000:8000", "8080:8080", "9999:9999", "9998:9998", "5173:5173", "5773:5773"},
		webProxyRoutes(px)...)

	database := composeService{
		Image: "mariadb:11.8",
		// Publish the database on a random loopback port so host-side tools
		// (e.g. `shopware-cli project sql`) can reach it without port conflicts.
		Ports: []string{"127.0.0.1::3306"},
		Environment: yamlMap[string]{}.
			set("MARIADB_DATABASE", "shopware").
			set("MARIADB_ROOT_PASSWORD", "root").
			set("MARIADB_USER", "shopware").
			set("MARIADB_PASSWORD", "shopware"),
		Volumes: []string{"db-data:/var/lib/mysql:rw"},
		Command: []string{
			"--sql_mode=STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION",
			"--log_bin_trust_function_creators=1",
			"--binlog_cache_size=16M",
			"--key_buffer_size=0",
			"--join_buffer_size=1024M",
			"--innodb_log_file_size=128M",
			"--innodb_buffer_pool_size=1024M",
			"--innodb_buffer_pool_instances=1",
			"--group_concat_max_len=320000",
			"--default-time-zone=+00:00",
			"--max_binlog_size=512M",
			"--binlog_expire_logs_seconds=86400",
		},
		Healthcheck: &composeHealthcheck{
			Test:          []string{"CMD", "mariadb-admin", "ping", "-h", "localhost", "-proot"},
			StartPeriod:   "10s",
			StartInterval: "3s",
			Interval:      "5s",
			Timeout:       "1s",
			Retries:       10,
		},
	}

	adminer := composeService{
		Image:      "adminer",
		StopSignal: "SIGKILL",
		// Long form with the default condition; equivalent to the short
		// "depends_on: [database]" the generator used to emit.
		DependsOn: yamlMap[composeDependency]{}.
			set("database", composeDependency{Condition: "service_started"}),
		Environment: yamlMap[string]{}.
			set("ADMINER_DEFAULT_SERVER", "database"),
	}
	publishOrRoute(&adminer, px, "adminer", []string{"9080:8080"}, proxyRoute{subdomain: "adminer", containerPort: 8080})

	mailer := composeService{
		Image: "axllent/mailpit",
		Environment: yamlMap[string]{}.
			set("MP_SMTP_AUTH_ACCEPT_ANY", "1").
			set("MP_SMTP_AUTH_ALLOW_INSECURE", "1"),
	}
	// Only the web UI (8025) is routed in proxy mode; SMTP (1025) stays internal
	// to the compose network, reachable by other services as mailer:1025.
	publishOrRoute(&mailer, px, "mailer", []string{"1025:1025", "8025:8025"}, proxyRoute{subdomain: "mailer", containerPort: 8025})

	services := yamlMap[composeService]{}.
		set("web", web).
		set("database", database).
		set("adminer", adminer).
		set("mailer", mailer)

	if opts != nil && opts.DedicatedWorker {
		// The admin no longer dispatches the queue or scheduled tasks from the
		// browser, so both need dedicated long-running processes. Each is
		// bounded by --time-limit / --memory-limit so it recycles periodically
		// (restart: unless-stopped brings it back up).
		for _, bg := range BackgroundServices {
			services = services.set(bg.Name, consoleService(opts, webEnv, webDependsOn, bg.command...))
		}
	}

	volumes := yamlMap[struct{}]{}.set("db-data", struct{}{})
	addOptionalServices(&services, &volumes, px, opts, features)

	file := composeFile{
		Services: services,
		Volumes:  volumes,
	}

	// In proxy mode every routed service joins the shared external network
	// Traefik also runs on, declared here so compose does not try to create it.
	if px != nil {
		file.Networks = yamlMap[composeExternalNetwork]{}.
			set(px.NetworkName, composeExternalNetwork{External: true})
	}

	return file
}

// consoleService builds a long-running service that reuses the web image and
// its environment to run a `php bin/console <args...>` process. It is used for
// the messenger worker and scheduled-task runner when the admin worker is
// disabled.
func consoleService(opts *ComposeOptions, webEnv yamlMap[string], webDependsOn yamlMap[composeDependency], consoleArgs ...string) composeService {
	svc := composeService{
		Image:       WebImage(opts),
		Command:     append([]string{"php", "bin/console"}, consoleArgs...),
		EnvFile:     []string{".env.local"},
		Environment: webEnv,
		DependsOn:   webDependsOn,
		Restart:     "unless-stopped",
	}
	if opts.User != "" {
		svc.User = opts.User
	}
	addVolumes(&svc, opts.proxy(), ".:/var/www/html")

	// The console processes (worker, scheduler) reach the shop's own APP_URL
	// over TLS, so in proxy mode they join the proxy network (the CA and
	// APP_URL env come from the shared webEnv). They publish no HTTP route.
	if px := opts.proxy(); px != nil {
		svc.Networks = []string{"default", px.NetworkName}
	}

	return svc
}

func baseWebEnv(px *ProxyOptions) yamlMap[string] {
	webEnv := yamlMap[string]{}.
		set("HOST", "0.0.0.0").
		set("DATABASE_URL", "mysql://root:root@database/shopware").
		set("MAILER_DSN", "smtp://mailer:1025")

	// In proxy mode Traefik terminates TLS and forwards plain HTTP from a
	// private container address, so Shopware must trust its X-Forwarded-*
	// headers (private ranges) or URL generation and redirects fall back to
	// http. Fixed-port mode only ever sees the local remote address.
	trustedProxies := "REMOTE_ADDR"
	if px != nil {
		trustedProxies = "127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}
	webEnv = webEnv.
		set("TRUSTED_PROXIES", trustedProxies).
		set("SYMFONY_TRUSTED_PROXIES", trustedProxies)

	if px != nil && px.CABundlePath != "" {
		// APP_URL is not pinned here: proxy up writes it into .env.local before
		// the container starts, keeping .env.local the single, editable source of
		// truth (a real env var would silently win over the file and confuse
		// anyone editing it). Point Node at the mounted CA bundle so its own
		// self-calls over TLS are trusted (PHP/curl trust it via the same bundle
		// mounted over the system trust store — see addVolumes).
		webEnv = webEnv.set("NODE_EXTRA_CA_CERTS", containerCABundlePath)
	}

	return webEnv
}

func applyLockEnv(webEnv yamlMap[string], px *ProxyOptions, features LockFeatures) (yamlMap[string], yamlMap[composeDependency]) {
	// Redis messenger wins over AMQP when the Redis transport is in the lock
	// (including via shopware/k8s-meta, which requires it).
	switch {
	case features.S3:
		webEnv = applyS3Env(webEnv, px)
	case features.RedisMessenger:
		webEnv = webEnv.set("MESSENGER_TRANSPORT_DSN", redisMessengerDSN)
	case features.AMQP:
		webEnv = webEnv.set("MESSENGER_TRANSPORT_DSN", "amqp://guest:guest@lavinmq:5672")
	}

	if features.Elasticsearch {
		webEnv = webEnv.
			set("OPENSEARCH_URL", "http://opensearch:9200").
			set("SHOPWARE_ES_ENABLED", "1").
			set("SHOPWARE_ES_INDEXING_ENABLED", "1").
			set("SHOPWARE_ES_INDEX_PREFIX", "sw")
	}

	webDependsOn := yamlMap[composeDependency]{}.
		set("database", composeDependency{Condition: "service_healthy"})
	if features.NeedsRedis() {
		webDependsOn = webDependsOn.set("redis", composeDependency{Condition: "service_healthy"})
	}
	if features.S3 {
		webDependsOn = webDependsOn.set("rustfs-init", composeDependency{Condition: "service_completed_successfully"})
	}

	return webEnv, webDependsOn
}

func applyProfilerEnv(webEnv yamlMap[string], opts *ComposeOptions) yamlMap[string] {
	if opts == nil || opts.PHPProfiler == "" {
		return webEnv
	}

	webEnv = webEnv.set("PHP_PROFILER", opts.PHPProfiler)
	switch opts.PHPProfiler {
	case "xdebug":
		webEnv = webEnv.
			set("XDEBUG_MODE", "debug").
			set("XDEBUG_CONFIG", "client_host=host.docker.internal")
	case "tideways":
		if opts.TidewaysAPIKey != "" {
			webEnv = webEnv.set("TIDEWAYS_APIKEY", opts.TidewaysAPIKey)
		}
	}

	return webEnv
}

func addOptionalServices(services *yamlMap[composeService], volumes *yamlMap[struct{}], px *ProxyOptions, opts *ComposeOptions, features LockFeatures) {
	if features.AMQP {
		lavinmq := composeService{Image: "cloudamqp/lavinmq"}
		// Only the management UI (15672) is routed in proxy mode; AMQP (5672)
		// stays internal, reachable as lavinmq:5672.
		publishOrRoute(&lavinmq, px, "lavinmq", []string{"15672:15672", "5672:5672"}, proxyRoute{subdomain: "lavinmq", containerPort: 15672})
		lavinmq.Volumes = []string{"lavinmq-data:/var/lib/lavinmq:rw"}
		*services = services.set("lavinmq", lavinmq)
		*volumes = volumes.set("lavinmq-data", struct{}{})
	}

	if features.Elasticsearch {
		opensearch := composeService{
			Image: "opensearchproject/opensearch:2",
			Environment: yamlMap[string]{}.
				set("OPENSEARCH_INITIAL_ADMIN_PASSWORD", "Shopware123!").
				set("discovery.type", "single-node").
				set("plugins.security.disabled", "true"),
		}
		publishOrRoute(&opensearch, px, "opensearch", []string{"9200:9200"}, proxyRoute{subdomain: "opensearch", containerPort: 9200})
		opensearch.Volumes = []string{"opensearch-data:/usr/share/opensearch/data"}
		*services = services.set("opensearch", opensearch)
		*volumes = volumes.set("opensearch-data", struct{}{})
	}

	if features.NeedsRedis() {
		addRedisService(services, volumes)
	}
	if features.S3 {
		addRustFSServices(services, volumes, px)
	}

	if opts != nil && opts.PHPProfiler == "blackfire" && opts.BlackfireServerID != "" && opts.BlackfireServerToken != "" {
		blackfire := composeService{
			Image: "blackfire/blackfire:2",
			Environment: yamlMap[string]{}.
				set("BLACKFIRE_SERVER_ID", opts.BlackfireServerID).
				set("BLACKFIRE_SERVER_TOKEN", opts.BlackfireServerToken),
		}
		*services = services.set("blackfire", blackfire)
	}

	if opts != nil && opts.PHPProfiler == "tideways" && opts.TidewaysAPIKey != "" {
		*services = services.set("tideways-daemon", composeService{Image: "ghcr.io/tideways/daemon"})
	}
}

// applyS3Env injects the filesystem, cache, session, and messenger values
// that shopware/k8s-meta's Flex recipe already reads. Compose environment
// overrides the recipe's localhost / empty-bucket defaults so PHP talks to the
// generated redis and rustfs services.
func applyS3Env(webEnv yamlMap[string], px *ProxyOptions) yamlMap[string] {
	// PHP talks to RustFS on the compose network. The browser loads public
	// media from PUBLIC_URL: localhost in plain mode, the s3.<host> proxy
	// route (HTTPS) when the shop itself is served through the local domain.
	publicURL := "http://127.0.0.1:9000/shopware-public"
	if px != nil {
		publicURL = "https://" + px.hostname(proxyRoute{subdomain: rustfsS3Subdomain}) + "/shopware-public"
	}

	return webEnv.
		set("K8S_FILESYSTEM_PRIVATE_BUCKET", "shopware-private").
		set("K8S_FILESYSTEM_PUBLIC_BUCKET", "shopware-public").
		set("K8S_FILESYSTEM_ENDPOINT", "http://rustfs:9000").
		set("K8S_FILESYSTEM_PUBLIC_URL", publicURL).
		set("K8S_FILESYSTEM_REGION", "us-east-1").
		set("AWS_ACCESS_KEY_ID", rustfsAccessKey).
		set("AWS_SECRET_ACCESS_KEY", rustfsSecretKey).
		set("AWS_DEFAULT_REGION", "us-east-1").
		set("K8S_CACHE_HOST", "redis").
		set("K8S_CACHE_PORT", "6379").
		set("PHP_SESSION_SAVE_PATH", "tcp://redis:6379").
		set("MESSENGER_TRANSPORT_DSN", redisMessengerDSN)
}

// addRedisService appends Redis when symfony/redis-messenger is in the lock
// (or shopware/k8s-meta, which requires it). It stays on the compose network
// only — PHP talks to redis:6379; nothing is published on the host.
func addRedisService(services *yamlMap[composeService], volumes *yamlMap[struct{}]) {
	redis := composeService{
		Image:   "redis:7-alpine",
		Volumes: []string{"redis-data:/data"},
		Healthcheck: &composeHealthcheck{
			Test:          []string{"CMD", "redis-cli", "ping"},
			StartPeriod:   "5s",
			StartInterval: "2s",
			Interval:      "5s",
			Timeout:       "1s",
			Retries:       10,
		},
	}

	*services = services.set("redis", redis)
	*volumes = volumes.set("redis-data", struct{}{})
}

// addRustFSServices appends RustFS and the one-shot bucket-init container that
// a PaaS lock (shopware/k8s-meta) needs. In plain mode S3 (9000) and the
// console (9001) are published on the host. In proxy mode they are routed at
// s3.<host> and rustfs.<host> so media URLs stay HTTPS on the local domain.
func addRustFSServices(services *yamlMap[composeService], volumes *yamlMap[struct{}], px *ProxyOptions) {
	rustfs := composeService{
		Image: "rustfs/rustfs:latest",
		Environment: yamlMap[string]{}.
			set("RUSTFS_VOLUMES", "/data").
			set("RUSTFS_ADDRESS", "0.0.0.0:9000").
			set("RUSTFS_CONSOLE_ADDRESS", "0.0.0.0:9001").
			set("RUSTFS_CONSOLE_ENABLE", "true").
			set("RUSTFS_ACCESS_KEY", rustfsAccessKey).
			set("RUSTFS_SECRET_KEY", rustfsSecretKey),
		Volumes: []string{"rustfs-data:/data"},
		Healthcheck: &composeHealthcheck{
			Test:          []string{"CMD", "curl", "-f", "http://127.0.0.1:9000/health"},
			StartPeriod:   "20s",
			StartInterval: "3s",
			Interval:      "5s",
			Timeout:       "5s",
			Retries:       10,
		},
	}
	publishOrRoute(&rustfs, px, "rustfs",
		[]string{"9000:9000", "9001:9001"},
		proxyRoute{subdomain: rustfsS3Subdomain, containerPort: 9000},
		proxyRoute{subdomain: rustfsConsoleSubdomain, containerPort: 9001},
	)

	rustfsInit := composeService{
		Image:      "minio/mc",
		Entrypoint: []string{"/bin/sh", "-c"},
		Command:    []string{fmt.Sprintf("mc alias set rustfs http://rustfs:9000 %s %s && mc mb --ignore-existing rustfs/shopware-private && mc mb --ignore-existing rustfs/shopware-public", rustfsAccessKey, rustfsSecretKey)},
		DependsOn: yamlMap[composeDependency]{}.
			set("rustfs", composeDependency{Condition: "service_healthy"}),
	}

	*services = services.set("rustfs", rustfs).set("rustfs-init", rustfsInit)
	*volumes = volumes.set("rustfs-data", struct{}{})
}
