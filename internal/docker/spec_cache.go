package docker

// buildCache builds the Redis instance used for the messenger transport,
// cache and sessions. It stays on the compose network only — PHP talks to
// cache:6379; nothing is published on the host.
func buildCache(*Environment, service) composeService {
	return composeService{
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
}
