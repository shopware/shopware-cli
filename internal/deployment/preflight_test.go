package deployment

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type previewBackend struct {
	Backend
	preview Preflight
}

func (b previewBackend) DeploymentPreflight(context.Context, Deployment, PreflightAction) (Preflight, error) {
	return b.preview, nil
}

func TestWritePreflightBackendFields(t *testing.T) {
	backend := previewBackend{preview: Preflight{
		ProjectDirectory: t.TempDir(),
		Targets: []PreflightTarget{
			{Fields: []PreflightField{
				{Label: "Cluster", Value: "production"},
				{Label: "Service", Value: "storefront\nworker"},
			}},
			{Fields: []PreflightField{{Label: "Cluster", Value: "staging"}}},
		},
	}}
	var output bytes.Buffer
	require.NoError(t, WritePreflight(t.Context(), backend, PreflightOptions{
		Action: PreflightRollout, Deployment: Deployment{Reference: "release"},
	}, &output))
	assert.Contains(t, output.String(), "\n  Cluster: production\n  Service: \"storefront\\nworker\"\n\n  Cluster: staging\n")
	assert.NotContains(t, output.String(), "SSH")
}
