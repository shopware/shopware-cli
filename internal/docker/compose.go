package docker

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func GenerateComposeFile(lock *packagist.ComposerLock) ([]byte, error) {
	hasAMQP := lock.GetPackage("symfony/amqp-messenger") != nil
	hasElasticsearch := lock.GetPackage("shopware/elasticsearch") != nil

	doc := buildCompose(hasAMQP, hasElasticsearch)

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, err
	}

	header := "# This file is managed by shopware-cli. Do not edit manually.\n" +
		"# Create a compose.override.yaml to customize services.\n" +
		"# See https://docs.docker.com/compose/how-tos/multiple-compose-files/merge/\n\n"

	return append([]byte(header), out...), nil
}

func WriteComposeFile(projectFolder string) error {
	lock, err := packagist.ReadComposerLock(filepath.Join(projectFolder, "composer.lock"))
	if err != nil {
		return fmt.Errorf("failed to read composer.lock: %w", err)
	}

	composeBytes, err := GenerateComposeFile(lock)
	if err != nil {
		return fmt.Errorf("failed to generate compose.yaml: %w", err)
	}

	return os.WriteFile(filepath.Join(projectFolder, "compose.yaml"), composeBytes, os.ModePerm)
}

func buildCompose(hasAMQP, hasElasticsearch bool) yaml.Node {
	webEnv := newMappingNode()
	addKeyValue(webEnv, "HOST", "0.0.0.0")
	addKeyValue(webEnv, "DATABASE_URL", "mysql://root:root@database/shopware")
	addKeyValue(webEnv, "MAILER_DSN", "smtp://mailer:1025")
	addKeyValue(webEnv, "TRUSTED_PROXIES", "REMOTE_ADDR")
	addKeyValue(webEnv, "SYMFONY_TRUSTED_PROXIES", "REMOTE_ADDR")

	if hasAMQP {
		addKeyValue(webEnv, "MESSENGER_TRANSPORT_DSN", "amqp://guest:guest@lavinmq:5672")
	}

	if hasElasticsearch {
		addKeyValue(webEnv, "OPENSEARCH_URL", "http://opensearch:9200")
		addKeyValue(webEnv, "SHOPWARE_ES_ENABLED", "1")
		addKeyValue(webEnv, "SHOPWARE_ES_INDEXING_ENABLED", "1")
		addKeyValue(webEnv, "SHOPWARE_ES_INDEX_PREFIX", "sw")
	}

	webDependsOn := newMappingNode()
	dbCondition := newMappingNode()
	addKeyValue(dbCondition, "condition", "service_healthy")
	addKeyValueNode(webDependsOn, "database", dbCondition)

	web := newMappingNode()
	addKeyValue(web, "image", "ghcr.io/shopware/docker-dev:php8.3-node22-caddy")
	addKeyValueNode(web, "ports", newSequenceNode(
		"8000:8000", "8080:8080", "9999:9999", "9998:9998", "5173:5173", "5773:5773",
	))
	addKeyValueNode(web, "env_file", newSequenceNode(".env.local"))
	addKeyValueNode(web, "environment", webEnv)
	addKeyValueNode(web, "volumes", newSequenceNode(".:/var/www/html"))
	addKeyValueNode(web, "depends_on", webDependsOn)

	dbEnv := newMappingNode()
	addKeyValue(dbEnv, "MARIADB_DATABASE", "shopware")
	addKeyValue(dbEnv, "MARIADB_ROOT_PASSWORD", "root")
	addKeyValue(dbEnv, "MARIADB_USER", "shopware")
	addKeyValue(dbEnv, "MARIADB_PASSWORD", "shopware")

	healthTest := newSequenceNode("CMD", "mariadb-admin", "ping", "-h", "localhost", "-proot")

	healthcheck := newMappingNode()
	addKeyValueNode(healthcheck, "test", healthTest)
	addKeyValue(healthcheck, "start_period", "10s")
	addKeyValue(healthcheck, "start_interval", "3s")
	addKeyValue(healthcheck, "interval", "5s")
	addKeyValue(healthcheck, "timeout", "1s")
	addKeyValueNode(healthcheck, "retries", &yaml.Node{Kind: yaml.ScalarNode, Value: "10", Tag: "!!int"})

	database := newMappingNode()
	addKeyValue(database, "image", "mariadb:11.8")
	addKeyValueNode(database, "environment", dbEnv)
	addKeyValueNode(database, "volumes", newSequenceNode("db-data:/var/lib/mysql:rw"))
	addKeyValueNode(database, "command", newSequenceNode(
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
	))
	addKeyValueNode(database, "healthcheck", healthcheck)

	adminerEnv := newMappingNode()
	addKeyValue(adminerEnv, "ADMINER_DEFAULT_SERVER", "database")

	adminer := newMappingNode()
	addKeyValue(adminer, "image", "adminer")
	addKeyValue(adminer, "stop_signal", "SIGKILL")
	addKeyValueNode(adminer, "depends_on", newSequenceNode("database"))
	addKeyValueNode(adminer, "environment", adminerEnv)
	addKeyValueNode(adminer, "ports", newSequenceNode("9080:8080"))

	mailerEnv := newMappingNode()
	addKeyValue(mailerEnv, "MP_SMTP_AUTH_ACCEPT_ANY", "1")
	addKeyValue(mailerEnv, "MP_SMTP_AUTH_ALLOW_INSECURE", "1")

	mailer := newMappingNode()
	addKeyValue(mailer, "image", "axllent/mailpit")
	addKeyValueNode(mailer, "ports", newSequenceNode("1025:1025", "8025:8025"))
	addKeyValueNode(mailer, "environment", mailerEnv)

	services := newMappingNode()
	addKeyValueNode(services, "web", web)
	addKeyValueNode(services, "database", database)
	addKeyValueNode(services, "adminer", adminer)
	addKeyValueNode(services, "mailer", mailer)

	volumes := newMappingNode()
	addKeyValueNode(volumes, "db-data", newNullNode())

	if hasAMQP {
		lavinmq := newMappingNode()
		addKeyValue(lavinmq, "image", "cloudamqp/lavinmq")
		addKeyValueNode(lavinmq, "ports", newSequenceNode("15672:15672", "5672:5672"))
		addKeyValueNode(lavinmq, "volumes", newSequenceNode("lavinmq-data:/var/lib/lavinmq:rw"))
		addKeyValueNode(services, "lavinmq", lavinmq)
		addKeyValueNode(volumes, "lavinmq-data", newNullNode())
	}

	if hasElasticsearch {
		osEnv := newMappingNode()
		addKeyValue(osEnv, "OPENSEARCH_INITIAL_ADMIN_PASSWORD", "Shopware123!")
		addKeyValue(osEnv, "discovery.type", "single-node")
		addKeyValue(osEnv, "plugins.security.disabled", "true")

		opensearch := newMappingNode()
		addKeyValue(opensearch, "image", "opensearchproject/opensearch:2")
		addKeyValueNode(opensearch, "environment", osEnv)
		addKeyValueNode(opensearch, "ports", newSequenceNode("9200:9200"))
		addKeyValueNode(opensearch, "volumes", newSequenceNode("opensearch-data:/usr/share/opensearch/data"))
		addKeyValueNode(services, "opensearch", opensearch)
		addKeyValueNode(volumes, "opensearch-data", newNullNode())
	}

	root := newMappingNode()
	addKeyValueNode(root, "services", services)
	addKeyValueNode(root, "volumes", volumes)

	return yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{root},
	}
}

// YAML node helpers to preserve insertion order.

func newMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func newSequenceNode(values ...string) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, v := range values {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: v, Tag: "!!str"})
	}
	return seq
}

func newNullNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}
}

func addKeyValue(m *yaml.Node, key, value string) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value, Tag: "!!str"},
	)
}

func addKeyValueNode(m *yaml.Node, key string, value *yaml.Node) {
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		value,
	)
}
