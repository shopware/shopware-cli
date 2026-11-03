package backend

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"charm.land/huh/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSelectEnvironment(t *testing.T) {
	cfg := &shop.Config{Environments: map[string]*shop.EnvironmentConfig{
		"staging": {Type: "ssh"}, "production": {Type: "ssh"},
		"local": {Type: "local"}, "docker": {Type: "docker"},
		"implicit-local": {}, "unknown": {Type: "unknown"}, "empty": nil,
	}}
	selected, err := selectEnvironment(t.Context(), cfg, func(names []string) (string, error) {
		assert.Equal(t, []string{"production", "staging"}, names)
		return names[1], nil
	})
	require.NoError(t, err)
	assert.Equal(t, "staging", selected)
}

func TestSelectEnvironmentStopsBeforeDeployment(t *testing.T) {
	for _, pickErr := range []error{huh.ErrUserAborted, errors.New("terminal unavailable")} {
		selected, err := selectEnvironment(t.Context(), &shop.Config{
			Environments: map[string]*shop.EnvironmentConfig{"production": {Type: "ssh"}},
		}, func([]string) (string, error) { return "production", pickErr })
		require.ErrorIs(t, err, pickErr)
		assert.Empty(t, selected)
	}
}

func TestSelectEnvironmentFiltersByTypeInsteadOfName(t *testing.T) {
	selected, err := selectEnvironment(t.Context(), &shop.Config{Environments: map[string]*shop.EnvironmentConfig{
		"local": {Type: "ssh"}, "production": {Type: "docker"},
	}}, func(names []string) (string, error) {
		require.Equal(t, []string{"local"}, names)
		return names[0], nil
	})
	require.NoError(t, err)
	assert.Equal(t, "local", selected)
}

func TestSelectEnvironmentWithoutChoices(t *testing.T) {
	_, err := selectEnvironment(t.Context(), &shop.Config{Environments: map[string]*shop.EnvironmentConfig{
		"local": {Type: "local"}, "docker": {Type: "docker"},
	}}, func([]string) (string, error) {
		t.Fatal("must not prompt without environments")
		return "", nil
	})
	require.Error(t, err)
}

func TestSelectEnvironmentCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := selectEnvironment(ctx, &shop.Config{}, func([]string) (string, error) {
		t.Fatal("must not prompt after cancellation")
		return "", nil
	})
	require.ErrorIs(t, err, context.Canceled)
}

func TestSelectEnvironmentNonInteractive(t *testing.T) {
	_, err := SelectEnvironment(system.WithInteraction(t.Context(), false), t.TempDir(), "", nil, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--env")
}

func TestSelectEnvironmentConfigErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "custom.yml")
	for _, contents := range []string{"", "environments: [invalid"} {
		testhelper.WriteFile(t, path, contents)
		_, err := SelectEnvironment(t.Context(), root, path, nil, io.Discard)
		require.Error(t, err)
	}
	_, err := SelectEnvironment(t.Context(), root, filepath.Join(root, "missing.yml"), nil, io.Discard)
	require.Error(t, err)
}
