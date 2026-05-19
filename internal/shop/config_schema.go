package shop

import _ "embed"

//go:embed config_schema.json
var configSchema []byte

// ConfigSchema returns the JSON schema for the .shopware-project.yml configuration file.
func ConfigSchema() []byte {
	return configSchema
}
