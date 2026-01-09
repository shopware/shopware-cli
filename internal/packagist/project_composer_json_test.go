package packagist

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateComposerJson(t *testing.T) {
	ctx := context.Background()

	t.Run("without audit", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0"})
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"sort-packages": true`)
		assert.NotContains(t, jsonStr, `"audit": {`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("with audit disabled", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0", NoAudit: true})
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"sort-packages": true`)
		assert.Contains(t, jsonStr, `"audit": {`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("with elasticsearch", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0", UseElasticsearch: true})
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"shopware/elasticsearch"`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("without elasticsearch", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0", UseElasticsearch: false})
		assert.NoError(t, err)
		assert.NotContains(t, jsonStr, `"shopware/elasticsearch"`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("with shopware paas deployment", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0", DeploymentMethod: DeploymentShopwarePaaS})
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"shopware/k8s-meta": "*"`)
		assert.NotContains(t, jsonStr, `"shopware/paas-meta"`)
		assert.Contains(t, jsonStr, `"platform": {`)
		assert.Contains(t, jsonStr, `"ext-grpc": "1.44.0"`)
		assert.Contains(t, jsonStr, `"ext-opentelemetry": "3.21.0"`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("with platformsh deployment", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0", DeploymentMethod: DeploymentPlatformSH})
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"shopware/paas-meta": "*"`)
		assert.NotContains(t, jsonStr, `"shopware/k8s-meta"`)
		assert.NotContains(t, jsonStr, `"ext-grpc"`)
		assert.NotContains(t, jsonStr, `"ext-opentelemetry"`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("with deployer deployment", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, ComposerJsonOptions{Version: "6.4.18.0", DeploymentMethod: DeploymentDeployer})
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"deployer/deployer": "*"`)
		assert.NotContains(t, jsonStr, `"shopware/paas-meta"`)
		assert.NotContains(t, jsonStr, `"shopware/k8s-meta"`)
		assert.NotContains(t, jsonStr, `"ext-grpc"`)
		assert.NotContains(t, jsonStr, `"ext-opentelemetry"`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})
}
