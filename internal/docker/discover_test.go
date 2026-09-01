package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func psContainer(service, state string, ports map[int]int) composePSContainer {
	c := composePSContainer{Name: "proj-" + service + "-1", Service: service, State: state}
	for target, published := range ports {
		c.Publishers = append(c.Publishers, composePSPublisher{TargetPort: target, PublishedPort: published})
	}
	return c
}

func TestClassifyContainersPlainMode(t *testing.T) {
	t.Parallel()

	env := classifyContainers([]composePSContainer{
		psContainer("web", "running", map[int]int{8000: 8002}),
		psContainer("database", "running", nil),
		psContainer("adminer", "running", map[int]int{8080: 9080}),
		psContainer("mailer", "running", map[int]int{1025: 1025, 8025: 8025}),
		psContainer("redis", "running", nil),
		psContainer("worker", "running", nil),
		psContainer("scheduler", "exited", nil),
		psContainer("rabbitmq", "running", map[int]int{15672: 15672}),
		psContainer("some-unknown", "running", map[int]int{1234: 1234}),
	}, "")

	assert.Equal(t, 8002, env.WebPort, "the web container's published shop port")

	assert.ElementsMatch(t, []DiscoveredService{
		{Name: "Adminer", URL: "http://127.0.0.1:9080", Username: "root", Password: "root"},
		{Name: "Mailpit", URL: "http://127.0.0.1:8025"},
		{Name: "Queue (RabbitMQ)", URL: "http://127.0.0.1:15672", Username: "guest", Password: "guest"},
	}, env.Services, "database is hidden, redis has no UI endpoint, unknown services are skipped")

	assert.ElementsMatch(t, []BackgroundProcess{
		{Name: "Queue worker", Running: true},
		{Name: "Scheduled tasks", Running: false},
	}, env.Background)
}

func TestClassifyContainersProxyMode(t *testing.T) {
	t.Parallel()

	env := classifyContainers([]composePSContainer{
		psContainer("web", "running", nil),
		psContainer("adminer", "running", nil),
		psContainer("rustfs", "running", nil),
		psContainer("rustfs-init", "running", nil),
		psContainer("rabbitmq", "running", nil),
	}, "my-shop.local")

	assert.Zero(t, env.WebPort, "proxied web publishes no host port")

	assert.ElementsMatch(t, []DiscoveredService{
		{Name: "Adminer", URL: "https://adminer.my-shop.local", Username: "root", Password: "root"},
		{Name: "S3 (RustFS)", URL: "https://rustfs.my-shop.local", Username: "shopware", Password: "shopware"},
	}, env.Services, "proxied services resolve to their subdomain; rustfs-init is hidden; overrides without a published port are skipped")
}

func TestClassifyContainersServiceWithoutReachableEndpoint(t *testing.T) {
	t.Parallel()

	// LavinMQ's UI endpoint publishes nothing and the project is not proxied.
	env := classifyContainers([]composePSContainer{
		psContainer("lavinmq", "running", map[int]int{5672: 5672}),
	}, "")

	assert.Empty(t, env.Services)
	assert.Zero(t, env.WebPort)
}
