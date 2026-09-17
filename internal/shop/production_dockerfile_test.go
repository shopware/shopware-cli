package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductionDockerfile(t *testing.T) {
	data, err := ProductionDockerfile(ProductionDockerfileOptions{
		PHPVersion: "8.4", WithDevDependencies: true,
	})
	require.NoError(t, err)
	text := string(data)
	assert.Contains(t, text, "#syntax=docker/dockerfile:1.10")
	assert.Contains(t, text, "ARG PHP_VERSION=8.4")
	assert.Contains(t, text, "docker-base:${PHP_VERSION}-frankenphp AS base-image")
	assert.Contains(t, text, "shopware-cli:latest-php-${PHP_VERSION} AS shopware-cli")
	assert.Contains(t, text, "id=packages_token,env=SHOPWARE_PACKAGES_TOKEN")
	assert.Contains(t, text, "id=composer_auth,dst=/src/auth.json")
	assert.NotContains(t, text, "ARG SHOPWARE_PACKAGES_TOKEN")
	assert.Contains(t, text, "project ci /src --with-dev-dependencies")
	assert.Contains(t, text, "COPY --from=build --chown=82 --link /src /var/www/html")
	assert.NotContains(t, text, "project_config")
	assert.NotContains(t, text, "SHOPWARE_CLI_CONFIG_HASH")

	data, err = ProductionDockerfile(ProductionDockerfileOptions{PHPVersion: "8.3"})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "--with-dev-dependencies")
	_, err = ProductionDockerfile(ProductionDockerfileOptions{PHPVersion: "8.3\nRUN false"})
	require.Error(t, err)
}
