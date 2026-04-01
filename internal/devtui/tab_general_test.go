package devtui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewGeneralModel(t *testing.T) {
	m := NewGeneralModel("docker", "http://localhost:8000", "admin", "shopware", "/tmp/project", nil)

	assert.Equal(t, "docker", m.envType)
	assert.Equal(t, "http://localhost:8000", m.shopURL)
	assert.Equal(t, "http://localhost:8000/admin", m.adminURL)
	assert.Equal(t, "admin", m.username)
	assert.Equal(t, "shopware", m.password)
	assert.Equal(t, "/tmp/project", m.projectRoot)
	assert.True(t, m.loading)
}

func TestNewGeneralModel_AdminURLTrailingSlash(t *testing.T) {
	m := NewGeneralModel("local", "http://localhost:8000/", "", "", "/tmp/project", nil)

	assert.Equal(t, "http://localhost:8000/admin", m.adminURL)
}

func TestNewGeneralModel_EmptyURL(t *testing.T) {
	m := NewGeneralModel("local", "", "", "", "/tmp/project", nil)

	assert.Equal(t, "admin", m.adminURL)
}

func TestServicesLoadedMsg(t *testing.T) {
	m := NewGeneralModel("docker", "http://localhost:8000", "", "", "/tmp/project", nil)

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

func TestServicesLoadedMsg_WithError(t *testing.T) {
	m := NewGeneralModel("docker", "http://localhost:8000", "", "", "/tmp/project", nil)

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

func TestViewShowsCredentials(t *testing.T) {
	m := NewGeneralModel("docker", "http://localhost:8000", "", "", "/tmp/project", nil)
	m.loading = false
	m.services = []DiscoveredService{
		{Name: "Adminer", URL: "http://127.0.0.1:9080", Username: "root", Password: "root"},
	}

	view := m.View(120, 40)
	assert.Contains(t, view, "Adminer")
	assert.Contains(t, view, "http://127.0.0.1:9080")
	assert.Contains(t, view, "root")
}
