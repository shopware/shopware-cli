package packagist

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateComposerJson(t *testing.T) {
	ctx := t.Context()

	t.Run("without audit", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, "6.4.18.0", false, false, false, false)
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"sort-packages": true`)
		assert.NotContains(t, jsonStr, `"audit": {`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})

	t.Run("with audit", func(t *testing.T) {
		jsonStr, err := GenerateComposerJson(ctx, "6.4.18.0", false, false, false, true)
		assert.NoError(t, err)
		assert.Contains(t, jsonStr, `"sort-packages": true`)
		assert.Contains(t, jsonStr, `"audit": {`)

		var data map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &data)
		assert.NoError(t, err, "Generated JSON should be valid")
	})
}
