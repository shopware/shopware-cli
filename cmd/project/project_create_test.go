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
