package extension

import _ "embed"

//go:embed config_schema.json
var configSchema []byte

// ConfigSchema returns the JSON schema for the .shopware-extension.yml configuration file.
func ConfigSchema() []byte {
	return configSchema
}
