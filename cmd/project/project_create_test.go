package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplyNonInteractiveDefaults(t *testing.T) {
	t.Parallel()

	t.Run("empty project folder defaults to current directory", func(t *testing.T) {
		t.Parallel()
		opts := createOptions{}
		err := applyNonInteractiveDefaults(&opts)
		assert.NoError(t, err)
		assert.Equal(t, ".", opts.projectFolder)
	})

	t.Run("dot project folder is kept", func(t *testing.T) {
		t.Parallel()
		opts := createOptions{projectFolder: "."}
		err := applyNonInteractiveDefaults(&opts)
		assert.NoError(t, err)
		assert.Equal(t, ".", opts.projectFolder)
	})

	t.Run("named project folder is kept", func(t *testing.T) {
		t.Parallel()
		opts := createOptions{projectFolder: "my-shop"}
		err := applyNonInteractiveDefaults(&opts)
		assert.NoError(t, err)
		assert.Equal(t, "my-shop", opts.projectFolder)
	})

	t.Run("--local-domain with --docker enables local domains without sudo", func(t *testing.T) {
		t.Parallel()
		opts := createOptions{useDocker: true, useLocalDomain: true}
		err := applyNonInteractiveDefaults(&opts)
		assert.NoError(t, err)
		assert.True(t, opts.useLocalDomain)
		// The one-time sudo setup is never run non-interactively.
		assert.False(t, opts.setupProxyNow)
	})

	t.Run("--local-domain without --docker is dropped", func(t *testing.T) {
		t.Parallel()
		opts := createOptions{useDocker: false, useLocalDomain: true}
		err := applyNonInteractiveDefaults(&opts)
		assert.NoError(t, err)
		assert.False(t, opts.useLocalDomain)
		assert.False(t, opts.setupProxyNow)
	})

	t.Run("no --local-domain keeps local domains off", func(t *testing.T) {
		t.Parallel()
		opts := createOptions{useDocker: true, useLocalDomain: false}
		err := applyNonInteractiveDefaults(&opts)
		assert.NoError(t, err)
		assert.False(t, opts.useLocalDomain)
		assert.False(t, opts.setupProxyNow)
	})
}

func TestIsComposerSecurityBlocked(t *testing.T) {
	t.Parallel()

	t.Run("detects packages blocked by security advisories", func(t *testing.T) {
		t.Parallel()
		output := `- shopware/core v6.7.12.1 requires dompdf/dompdf 3.1.4 -> found dompdf/dompdf[v3.1.4] but these were not loaded, because they are affected by security advisories ("PKSA-cv56-2228-pzqx")`
		assert.True(t, isComposerSecurityBlocked(output))
	})

	t.Run("ignores regular resolution conflicts", func(t *testing.T) {
		t.Parallel()
		output := `- shopware/administration v6.7.6.0 requires shopware/core v6.7.6.0 -> found shopware/core[v6.7.6.0] but it conflicts with your root composer.json require (6.7.12.1).`
		assert.False(t, isComposerSecurityBlocked(output))
	})

	t.Run("ignores empty output", func(t *testing.T) {
		t.Parallel()
		assert.False(t, isComposerSecurityBlocked(""))
	})
}

func TestHandleSecurityBlockedInstallNonInteractive(t *testing.T) {
	t.Parallel()

	opts := createOptions{projectFolder: t.TempDir(), interactive: false}

	err := handleSecurityBlockedInstall(t.Context(), &opts, "6.7.12.1")

	assert.ErrorContains(t, err, "re-run with --no-audit")
	assert.ErrorContains(t, err, "6.7.12.1")
	assert.False(t, opts.noAudit)
}
