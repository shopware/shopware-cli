package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSettings(t *testing.T) {
	t.Parallel()

	assert.NoError(t, ValidateSettings(nil))
	assert.NoError(t, ValidateSettings(Settings{
		ServiceAdminer:  nil,
		ServiceDatabase: {Type: DatabaseMySQL, Version: "8.4", Ports: Ports{PortMySQL: 3306}},
		ServiceWeb:      {Ports: Ports{PortHTTP: 8005}},
	}))

	assert.ErrorContains(t, ValidateSettings(Settings{"postgres": {}}),
		"docker.services.postgres: unknown service, valid services: web, database, adminer, mailer, queue, search, storage")
	assert.ErrorContains(t, ValidateSettings(Settings{ServiceCache: {}}),
		"docker.services.cache: unknown service")
	assert.ErrorContains(t, ValidateSettings(Settings{ServiceMailer: {Ports: Ports{"web": 8025}}}),
		"docker.services.mailer.ports.web: unknown port, valid ports: smtp, http")
	assert.ErrorContains(t, ValidateSettings(Settings{ServiceMailer: {Type: "postfix"}}),
		"docker.services.mailer: type and version are not configurable")
	assert.ErrorContains(t, ValidateSettings(Settings{ServiceDatabase: {Type: "postgres"}}),
		`docker.services.database.type: unknown value "postgres", valid values: mariadb, mysql`)
}

func TestSettingsSchema(t *testing.T) {
	t.Parallel()

	schema := SettingsSchema()
	require.NotNil(t, schema.Properties)
	assert.Equal(t, "object", schema.Type)

	var names []string
	for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
		names = append(names, pair.Key)
	}
	assert.Equal(t, []string{"web", "database", "adminer", "mailer", "queue", "search", "storage"}, names)

	database, _ := schema.Properties.Get("database")
	typeSchema, ok := database.Properties.Get("type")
	require.True(t, ok)
	assert.Equal(t, []any{"mariadb", "mysql"}, typeSchema.Enum)
	_, ok = database.Properties.Get("version")
	assert.True(t, ok)
	ports, _ := database.Properties.Get("ports")
	mysql, _ := ports.Properties.Get("mysql")
	assert.Contains(t, mysql.Description, "random port")

	mailer, _ := schema.Properties.Get("mailer")
	_, ok = mailer.Properties.Get("type")
	assert.False(t, ok, "services without variants accept no type")

	web, _ := schema.Properties.Get("web")
	webPorts, _ := web.Properties.Get("ports")
	http, _ := webPorts.Properties.Get("http")
	assert.Equal(t, "#/$defs/Port", http.Ref)
	assert.Contains(t, http.Description, "Defaults to 8000.")
}
