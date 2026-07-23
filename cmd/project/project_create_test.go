package project

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestProjectCreateRejectsInvalidNameArgument(t *testing.T) {
	// A name provided directly as an argument must be rejected up front,
	// before the interactive form or any network call, the same way the
	// interactive name prompt rejects it live.
	invalidNames := []string{"myShop", "MyShop", "müller", "my shop"}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			projectCreateCmd.SetContext(t.Context())
			err := projectCreateCmd.RunE(projectCreateCmd, []string{name})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid project name")
		})
	}
}

func TestProjectNameFieldDescription(t *testing.T) {
	t.Parallel()

	t.Run("empty shows help text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, projectNameHelp, projectNameFieldDescription(""))
	})

	t.Run("valid name shows help text", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, projectNameHelp, projectNameFieldDescription("my-shop"))
	})

	t.Run("uppercase name shows the rule", func(t *testing.T) {
		t.Parallel()
		desc := projectNameFieldDescription("MyShop")
		assert.NotEqual(t, projectNameHelp, desc)
		assert.Contains(t, desc, shop.ProjectNameRule)
	})

	t.Run("umlaut name shows the rule", func(t *testing.T) {
		t.Parallel()
		desc := projectNameFieldDescription("müller")
		assert.NotEqual(t, projectNameHelp, desc)
		assert.Contains(t, desc, shop.ProjectNameRule)
	})
}

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
