package docker

// containerCABundlePath is where the combined CA bundle is mounted inside the
// shop's containers: straight over the system trust store, so openssl, curl and
// PHP pick it up with no update-ca-certificates step (which the www-data image
// user could not run anyway). NODE_EXTRA_CA_CERTS points Node at the same file.
const containerCABundlePath = "/etc/ssl/certs/ca-certificates.crt"

// redisMessengerDSN is the Symfony Redis transport used when
// symfony/redis-messenger is in the lock (shopware/k8s-meta requires it). It
// points at the generated cache service (not localhost) so PHP containers can
// reach the queue on the compose network. If both Redis and AMQP messengers
// are present, this DSN wins over the AMQP broker.
const redisMessengerDSN = "redis://cache:6379/messages/symfony/?auto_setup=true&serializer=1&stream_max_entries=0&dbindex=0"

// consoleProcessLimits bound the dedicated console processes so they recycle
// periodically (paired with restart: unless-stopped in the compose file).
var consoleProcessLimits = []string{"--time-limit=300", "--memory-limit=512M"}

// buildWeb builds the shop container: the docker-dev image with the project
// bind-mounted, wired to every generated service through its environment.
func buildWeb(e *Environment, _ service) composeService {
	web := composeService{
		Image:       e.WebImage(),
		User:        e.user,
		EnvFile:     []string{".env.local"},
		Environment: webEnvironment(e),
		Volumes:     phpVolumes(e),
		DependsOn:   webDependencies(e),
	}

	return web
}

// buildConsole returns the build function for a long-running
// `php bin/console <args...>` process that reuses the web image and its
// environment: the messenger worker and scheduled-task runner when the admin
// worker is disabled. Each is bounded by --time-limit / --memory-limit so it
// recycles periodically (restart: unless-stopped brings it back up).
func buildConsole(consoleArgs ...string) func(*Environment, service) composeService {
	command := append([]string{"php", "bin/console"}, consoleArgs...)
	command = append(command, consoleProcessLimits...)

	return func(e *Environment, svc service) composeService {
		console := buildWeb(e, svc)
		console.Command = command
		console.Restart = "unless-stopped"

		// The console processes reach the shop's own APP_URL over TLS, so in
		// proxy mode they join the proxy network (the CA and APP_URL env come
		// from the shared web environment). They publish no HTTP route.
		if e.proxy != nil {
			console.Networks = proxyNetworks(e.proxy.NetworkName, "")
		}

		return console
	}
}

// phpVolumes bind-mounts the project, plus the read-only combined CA bundle
// over the system trust store in proxy mode — so code in the container (PHP,
// curl, Node) trusts the proxy's HTTPS certificates for self-calls to APP_URL
// while still trusting public CAs.
func phpVolumes(e *Environment) []string {
	volumes := []string{".:/var/www/html"}
	if e.proxy != nil && e.proxy.CABundlePath != "" {
		volumes = append(volumes, e.proxy.CABundlePath+":"+containerCABundlePath+":ro")
	}

	return volumes
}

func webEnvironment(e *Environment) yamlMap[string] {
	// Services on the shared proxy network are addressed by their project-unique
	// alias (see serviceHost); the database stays on the project network, so
	// its bare name never collides.
	webEnv := yamlMap[string]{}.
		set("HOST", "0.0.0.0").
		set("DATABASE_URL", "mysql://root:root@database/shopware").
		set("MAILER_DSN", "smtp://"+e.serviceHost(ServiceMailer)+":1025")

	// In proxy mode Traefik terminates TLS and forwards plain HTTP from a
	// private container address, so Shopware must trust its X-Forwarded-*
	// headers (private ranges) or URL generation and redirects fall back to
	// http. Fixed-port mode only ever sees the local remote address.
	trustedProxies := "REMOTE_ADDR"
	if e.proxy != nil {
		trustedProxies = "127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"
	}
	webEnv = webEnv.
		set("TRUSTED_PROXIES", trustedProxies).
		set("SYMFONY_TRUSTED_PROXIES", trustedProxies)

	if e.proxy != nil && e.proxy.CABundlePath != "" {
		// APP_URL is not pinned here: proxy up writes it into .env.local before
		// the container starts, keeping .env.local the single, editable source of
		// truth (a real env var would silently win over the file and confuse
		// anyone editing it). Point Node at the mounted CA bundle so its own
		// self-calls over TLS are trusted (PHP/curl trust it via the same bundle
		// mounted over the system trust store — see phpVolumes).
		webEnv = webEnv.set("NODE_EXTRA_CA_CERTS", containerCABundlePath)
	}

	// Redis messenger wins over AMQP when the Redis transport is in the lock
	// (including via shopware/k8s-meta, which requires it).
	switch {
	case e.features.S3:
		webEnv = applyS3Env(webEnv, e)
	case e.features.RedisMessenger:
		webEnv = webEnv.set("MESSENGER_TRANSPORT_DSN", redisMessengerDSN)
	case e.features.AMQP:
		webEnv = webEnv.set("MESSENGER_TRANSPORT_DSN", "amqp://guest:guest@"+e.serviceHost(ServiceQueue)+":5672")
	}

	if e.features.Elasticsearch {
		webEnv = webEnv.
			set("OPENSEARCH_URL", "http://"+e.serviceHost(ServiceSearch)+":9200").
			set("SHOPWARE_ES_ENABLED", "1").
			set("SHOPWARE_ES_INDEXING_ENABLED", "1").
			set("SHOPWARE_ES_INDEX_PREFIX", "sw")
	}

	return applyProfilerEnv(webEnv, e)
}

// applyS3Env injects the filesystem, cache, session, elasticsearch, and
// messenger values that shopware/k8s-meta's Flex recipe already reads. Compose
// environment overrides the recipe's localhost / empty-bucket defaults so PHP
// talks to the generated cache and storage services.
func applyS3Env(webEnv yamlMap[string], e *Environment) yamlMap[string] {
	// PHP talks to the storage on the compose network. The browser loads
	// public media from PUBLIC_URL: the published host port in plain mode, the
	// s3.<host> proxy route (HTTPS) when the shop is served through the local
	// domain.
	publicURL := storageS3.url(e.proxyHost(), e.ports(ServiceStorage))
	if publicURL == "" {
		// A disabled S3 port leaves no host-reachable endpoint; fall back to
		// the default so the generated env stays syntactically valid.
		publicURL = loopbackURL(storageS3.DefaultHostPort)
	}
	publicURL += "/shopware-public"

	return webEnv.
		set("K8S_FILESYSTEM_PRIVATE_BUCKET", "shopware-private").
		set("K8S_FILESYSTEM_PUBLIC_BUCKET", "shopware-public").
		set("K8S_FILESYSTEM_ENDPOINT", "http://"+e.serviceHost(ServiceStorage)+":9000").
		set("K8S_FILESYSTEM_PUBLIC_URL", publicURL).
		set("K8S_FILESYSTEM_REGION", "us-east-1").
		set("AWS_ACCESS_KEY_ID", storageAccessKey).
		set("AWS_SECRET_ACCESS_KEY", storageSecretKey).
		set("AWS_DEFAULT_REGION", "us-east-1").
		set("K8S_CACHE_HOST", "cache").
		set("K8S_CACHE_PORT", "6379").
		set("PHP_SESSION_HANDLER", "redis").
		set("PHP_SESSION_SAVE_PATH", "tcp://cache:6379").
		set("K8S_ES_NUMBER_OF_REPLICAS", "1").
		set("K8S_ES_NUMBER_OF_SHARDS", "1").
		set("MESSENGER_TRANSPORT_DSN", redisMessengerDSN)
}

func applyProfilerEnv(webEnv yamlMap[string], e *Environment) yamlMap[string] {
	if e.php.Profiler == "" {
		return webEnv
	}

	webEnv = webEnv.set("PHP_PROFILER", e.php.Profiler)
	switch e.php.Profiler {
	case ProfilerXdebug:
		webEnv = webEnv.
			set("XDEBUG_MODE", "debug").
			set("XDEBUG_CONFIG", "client_host=host.docker.internal")
	case ProfilerTideways:
		if e.php.TidewaysAPIKey != "" {
			webEnv = webEnv.set("TIDEWAYS_APIKEY", e.php.TidewaysAPIKey)
		}
	}

	return webEnv
}

func webDependencies(e *Environment) yamlMap[composeDependency] {
	dependsOn := yamlMap[composeDependency]{}.
		set(ServiceDatabase, composeDependency{Condition: "service_healthy"})
	if e.features.needsRedis() {
		dependsOn = dependsOn.set(ServiceCache, composeDependency{Condition: "service_healthy"})
	}
	if e.features.S3 {
		dependsOn = dependsOn.set(ServiceStorageInit, composeDependency{Condition: "service_completed_successfully"})
	}

	return dependsOn
}
