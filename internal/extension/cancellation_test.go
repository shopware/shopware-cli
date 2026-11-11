package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
)

type cancelingCheck struct {
	testCheck
	cancel context.CancelFunc
}

func (c *cancelingCheck) AddResult(result validation.CheckResult) {
	c.testCheck.AddResult(result)
	c.cancel()
}

func TestValidationStopsAfterCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".DS_Store"), nil, 0o644))
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	check := &cancelingCheck{cancel: cancel}
	validateZipFiles(ctx, root, check)
	assert.Len(t, check.Results, 1, "the walk should stop after the first finding cancels it")

	check.Results = nil
	RunValidation(ctx, getTestPlugin(root), check, "8.2")
	assert.Empty(t, check.Results, "canceled validation should not start any stage")
}

func TestSnippetDiscoveryStopsAfterCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"en.json", "de.json"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("{}"), 0o644))
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	visited := 0
	err := validateSnippetsByPath(ctx, root, root, &testCheck{}, func(string, string) bool {
		visited++
		cancel()
		return true
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, visited)
}
