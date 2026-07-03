package devtui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMajorMinor(t *testing.T) {
	assert.Equal(t, "6.7", majorMinor("6.7.0.0"))
	assert.Equal(t, "6.6", majorMinor("6.6.10.19"))
	assert.Equal(t, "6.7", majorMinor("6.7"))
	assert.Equal(t, "", majorMinor("6"))
	assert.Equal(t, "", majorMinor(""))
}

func TestSecurityEndLevel(t *testing.T) {
	now := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, securityEndOK, securityEndLevel(now.AddDate(2, 0, 0), now))
	assert.Equal(t, securityEndWarning, securityEndLevel(now.AddDate(0, 6, 0), now))
	assert.Equal(t, securityEndCritical, securityEndLevel(now.AddDate(0, 0, 10), now))
	assert.Equal(t, securityEndCritical, securityEndLevel(now.AddDate(0, 0, -1), now))
}

func TestSecurityEndRemaining(t *testing.T) {
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)

	assert.Equal(t, "607 days left", securityEndRemaining(time.Date(2028, 2, 28, 0, 0, 0, 0, time.UTC), now))
	assert.Equal(t, "1 day left", securityEndRemaining(now.Add(36*time.Hour), now))
	assert.Equal(t, "expires today", securityEndRemaining(now.Add(12*time.Hour), now))
	assert.Equal(t, "expired", securityEndRemaining(now.Add(-1*time.Hour), now))
}

func TestOverviewBackgroundProcessesSection(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil, nil)
	m.loading = false
	m.width = 80
	m.height = 40

	// Hidden entirely when there are no background processes.
	assert.NotContains(t, m.View(m.width, m.height), "Background processing")

	m.background = []BackgroundProcess{
		{Name: "Queue worker", Running: true},
		{Name: "Scheduled tasks", Running: false},
	}

	for _, width := range []int{120, 80} { // two-column and stacked layouts
		view := m.View(width, m.height)
		assert.Contains(t, view, "Background processing")
		assert.Contains(t, view, "Queue worker")
		assert.Contains(t, view, "running")
		assert.Contains(t, view, "Scheduled tasks")
		assert.Contains(t, view, "stopped")
		// Background processes use the same status dots as Setup health.
		assert.Contains(t, view, "●")
	}
}

func TestNewOverviewModel(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil, nil)

	assert.Equal(t, "docker", m.envType)
	assert.Equal(t, "http://localhost:8000", m.shopURL)
	assert.Equal(t, "http://localhost:8000/admin", m.adminURL)
	assert.Equal(t, "admin", m.username)
	assert.Equal(t, "shopware", m.password)
	assert.Equal(t, "/tmp/project", m.projectRoot)
	assert.True(t, m.loading)
}

func TestNewOverviewModel_AdminURLTrailingSlash(t *testing.T) {
	m := NewOverviewModel("local", "http://localhost:8000/", "", "", "/tmp/project", nil, nil)

	assert.Equal(t, "http://localhost:8000/admin", m.adminURL)
}

func TestNewOverviewModel_EmptyURL(t *testing.T) {
	m := NewOverviewModel("local", "", "", "", "/tmp/project", nil, nil)

	assert.Equal(t, "admin", m.adminURL)
}

func TestServicesLoadedMsg(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "", "", "/tmp/project", nil, nil)

	services := []DiscoveredService{
		{Name: "Adminer", URL: "http://127.0.0.1:9080", Username: "root", Password: "root"},
		{Name: "Shopware", URL: "http://localhost:8000"},
	}

	updated, cmd := m.Update(servicesLoadedMsg{services: services})
	assert.Nil(t, cmd)
	assert.False(t, updated.loading)
	assert.Nil(t, updated.err)
	assert.Len(t, updated.services, 2)
	assert.Equal(t, "Adminer", updated.services[0].Name)
	assert.Equal(t, "root", updated.services[0].Username)
	assert.Equal(t, "Shopware", updated.services[1].Name)
}

func TestServicesLoadedMsg_UpdatesPortFromWebService(t *testing.T) {
	m := NewOverviewModel("docker", "http://127.0.0.1:8000", "", "", "/tmp/project", nil, nil)

	updated, _ := m.Update(servicesLoadedMsg{webPort: 8002})
	assert.Equal(t, "http://127.0.0.1:8002", updated.shopURL)
	assert.Equal(t, "http://127.0.0.1:8002/admin", updated.adminURL)
}

func TestServicesLoadedMsg_KeepsCustomHostWhenUpdatingPort(t *testing.T) {
	m := NewOverviewModel("docker", "http://foo.localhost:8000", "", "", "/tmp/project", nil, nil)

	updated, _ := m.Update(servicesLoadedMsg{webPort: 8002})
	assert.Equal(t, "http://foo.localhost:8002", updated.shopURL)
	assert.Equal(t, "http://foo.localhost:8002/admin", updated.adminURL)
}

func TestServicesLoadedMsg_NoWebPortKeepsURL(t *testing.T) {
	m := NewOverviewModel("docker", "http://127.0.0.1:8000", "", "", "/tmp/project", nil, nil)

	updated, _ := m.Update(servicesLoadedMsg{})
	assert.Equal(t, "http://127.0.0.1:8000", updated.shopURL)
	assert.Equal(t, "http://127.0.0.1:8000/admin", updated.adminURL)
}

func TestResolveShopURL(t *testing.T) {
	assert.Equal(t, "http://127.0.0.1:8002", ResolveShopURL("http://127.0.0.1:8000", 8002))
	assert.Equal(t, "http://foo.localhost:8002", ResolveShopURL("http://foo.localhost:8000", 8002))
	// URL without an explicit port still gets the discovered port applied.
	assert.Equal(t, "http://foo.localhost:8002", ResolveShopURL("http://foo.localhost", 8002))
	// HTTPS and paths are preserved.
	assert.Equal(t, "https://shop.example.com:8443/", ResolveShopURL("https://shop.example.com/", 8443))

	// Unchanged when there is nothing to resolve.
	assert.Equal(t, "", ResolveShopURL("", 8002))
	assert.Equal(t, "http://127.0.0.1:8000", ResolveShopURL("http://127.0.0.1:8000", 0))
}

func TestServicesLoadedMsg_WithError(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "", "", "/tmp/project", nil, nil)

	updated, _ := m.Update(servicesLoadedMsg{err: assert.AnError})
	assert.False(t, updated.loading)
	assert.Error(t, updated.err)
	assert.Empty(t, updated.services)
}

func TestKnownServices(t *testing.T) {
	adminer := knownServices["adminer"]
	assert.Equal(t, "Adminer", adminer.Name)
	assert.Equal(t, 8080, adminer.TargetPort)
	assert.Equal(t, "root", adminer.Username)

	mailer := knownServices["mailer"]
	assert.Equal(t, "Mailpit", mailer.Name)
	assert.Equal(t, 8025, mailer.TargetPort)

	lavinmq := knownServices["lavinmq"]
	assert.Equal(t, "Queue (LavinMQ)", lavinmq.Name)
	assert.Equal(t, 15672, lavinmq.TargetPort)
	assert.Equal(t, "guest", lavinmq.Username)

	rabbitmq := knownServices["rabbitmq"]
	assert.Equal(t, "Queue (RabbitMQ)", rabbitmq.Name)
	assert.Equal(t, 15672, rabbitmq.TargetPort)
}

func TestViewShowsAccessTable(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil, nil)
	m.loading = false
	m.services = []DiscoveredService{
		{Name: "Adminer", URL: "http://127.0.0.1:9080", Username: "root", Password: "root"},
		{Name: "Mailpit", URL: "http://127.0.0.1:8025"},
	}

	for _, width := range []int{120, 80} { // two-column and stacked layouts
		view := m.View(width, 40)
		assert.Contains(t, view, "Access")
		assert.Contains(t, view, "Password / Auth")
		assert.Contains(t, view, "Shop Admin")
		assert.Contains(t, view, "http://localhost:8000/admin")
		assert.Contains(t, view, "shopware")
		assert.Contains(t, view, "Adminer")
		assert.Contains(t, view, "http://127.0.0.1:9080")
		assert.Contains(t, view, "root")
		// Services without credentials are marked as open.
		assert.Contains(t, view, "no auth")
	}
}

func TestViewAccessTableWithoutInstallation(t *testing.T) {
	m := NewOverviewModel("docker", "http://localhost:8000", "", "", "/tmp/project", nil, nil)
	m.loading = false

	view := m.View(120, 40)
	assert.Contains(t, view, "Shop Admin")
	assert.Contains(t, view, "Admin credentials will appear here once Shopware is installed.")
	assert.NotContains(t, view, "no auth")
}
