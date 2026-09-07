package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoutedSubdomains(t *testing.T) {
	t.Parallel()

	base := &Environment{}
	assert.Equal(t,
		[]string{"", subdomainAdminWatch, subdomainStorefrontWatch, "adminer", "mailer"},
		base.RoutedSubdomains())

	full := &Environment{features: features{AMQP: true, Elasticsearch: true, S3: true}}
	assert.Equal(t,
		[]string{"", subdomainAdminWatch, subdomainStorefrontWatch, "adminer", "mailer", "queue", "search", "s3", "storage"},
		full.RoutedSubdomains())
}

func TestServiceSubdomains(t *testing.T) {
	t.Parallel()

	// Web's custom routes share subdomains across several routes; they are
	// reported once each, root first.
	assert.Equal(t, []string{"", subdomainAdminWatch, subdomainStorefrontWatch}, serviceSubdomains(*byName(ServiceWeb)))
	// Endpoint-derived: only endpoints with a subdomain are routed.
	assert.Equal(t, []string{"mailer"}, serviceSubdomains(*byName(ServiceMailer)))
	assert.Equal(t, []string{"s3", "storage"}, serviceSubdomains(*byName(ServiceStorage)))
	assert.Empty(t, serviceSubdomains(*byName(ServiceDatabase)))
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	plain := &Environment{}
	assert.Empty(t, plain.routes(*byName(ServiceMailer)), "nothing is routed in fixed-port mode")

	proxied := &Environment{proxy: &Proxy{Hostname: "my-shop.shopware.local"}}
	assert.Len(t, proxied.routes(*byName(ServiceMailer)), 1)
	assert.Empty(t, proxied.routes(*byName(ServiceDatabase)), "the database has no proxy route in either mode")
}

func TestPortBindings(t *testing.T) {
	t.Parallel()

	mailer := *byName(ServiceMailer)
	assert.Equal(t, []string{"1025:1025", "8025:8025"}, portBindings(mailer, nil))
	assert.Equal(t, []string{"1025:1025", "18025:8025"}, portBindings(mailer, Ports{PortHTTP: 18025}))
	assert.Equal(t, []string{"8025:8025"}, portBindings(mailer, Ports{PortSMTP: PortDisabled}))

	// Nameless endpoints are never published by the generator.
	assert.Empty(t, portBindings(*byName(ServiceCache), nil))

	// A loopback endpoint without a default publishes on a random loopback
	// port until pinned.
	database := *byName(ServiceDatabase)
	assert.Equal(t, []string{"127.0.0.1::3306"}, portBindings(database, nil))
	assert.Equal(t, []string{"127.0.0.1:3307:3306"}, portBindings(database, Ports{PortMySQL: 3307}))
	assert.Empty(t, portBindings(database, Ports{PortMySQL: PortDisabled}))

	// The queue broker and the search node are published on loopback only, so
	// they are reachable from the host but not from the network.
	assert.Equal(t, []string{"127.0.0.1:15672:15672", "127.0.0.1:5672:5672"}, portBindings(*byName(ServiceQueue), nil))
	assert.Equal(t, []string{"127.0.0.1:19200:9200"}, portBindings(*byName(ServiceSearch), Ports{PortHTTP: 19200}))
}
