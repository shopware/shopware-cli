package docker

// buildAdminer builds the database UI.
func buildAdminer(*Environment, service) composeService {
	return composeService{
		Image:      "adminer",
		StopSignal: "SIGKILL",
		Environment: yamlMap[string]{}.
			set("ADMINER_DEFAULT_SERVER", ServiceDatabase),
		// Long form with the default condition; equivalent to the short
		// "depends_on: [database]" the generator used to emit.
		DependsOn: yamlMap[composeDependency]{}.
			set(ServiceDatabase, composeDependency{Condition: "service_started"}),
	}
}

// buildMailer builds the Mailpit catch-all mailer. Only its web UI is routed
// in proxy mode; SMTP stays internal to the compose network, reachable by
// other services as mailer:1025.
func buildMailer(*Environment, service) composeService {
	return composeService{
		Image: "axllent/mailpit",
		Environment: yamlMap[string]{}.
			set("MP_SMTP_AUTH_ACCEPT_ANY", "1").
			set("MP_SMTP_AUTH_ALLOW_INSECURE", "1"),
	}
}

// buildBlackfire builds the Blackfire agent the PHP probe reports to.
func buildBlackfire(e *Environment, _ service) composeService {
	return composeService{
		Image: "blackfire/blackfire:2",
		Environment: yamlMap[string]{}.
			set("BLACKFIRE_SERVER_ID", e.php.BlackfireServerID).
			set("BLACKFIRE_SERVER_TOKEN", e.php.BlackfireServerToken),
	}
}

// buildTideways builds the Tideways daemon the PHP extension reports to.
func buildTideways(*Environment, service) composeService {
	return composeService{Image: "ghcr.io/tideways/daemon"}
}
