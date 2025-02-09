package extension

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnippetValidateNoExistingFolderAdmin(t *testing.T) {
	context := newValidationContext(PlatformPlugin{
		path:   "test",
		config: &Config{},
	})

	validateAdministrationSnippets(context)
}

func TestSnippetValidateNoExistingFolderStorefront(t *testing.T) {
	context := newValidationContext(PlatformPlugin{
		path:   "test",
		config: &Config{},
	})

	validateAdministrationSnippets(context)
}

func TestSnippetValidateStorefrontByPathOneFileIsIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	context := newValidationContext(PlatformPlugin{
		path:   tmpDir,
		config: &Config{},
	})

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{}`), os.ModePerm)

	assert.NoError(t, validateStorefrontSnippetsByPath(tmpDir, tmpDir, context))
	assert.Len(t, context.errors, 0)
	assert.Len(t, context.warnings, 0)
}

func TestSnippetValidateStorefrontByPathSameFile(t *testing.T) {
	tmpDir := t.TempDir()

	context := newValidationContext(PlatformPlugin{
		path:   tmpDir,
		config: &Config{},
	})

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"test": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"test": "2"}`), os.ModePerm)

	assert.NoError(t, validateStorefrontSnippetsByPath(tmpDir, tmpDir, context))
	assert.Len(t, context.errors, 0)
	assert.Len(t, context.warnings, 0)
}

func TestSnippetValidateStorefrontByPathTestDifferent(t *testing.T) {
	tmpDir := t.TempDir()

	context := newValidationContext(PlatformPlugin{
		path:   tmpDir,
		config: &Config{},
	})

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"b": "2"}`), os.ModePerm)

	assert.NoError(t, validateStorefrontSnippetsByPath(tmpDir, tmpDir, context))
	assert.Len(t, context.errors, 0)
	assert.Len(t, context.warnings, 2)
	assert.Contains(t, context.warnings[0].Message, "key /a is missing, but defined in the main language file")
	assert.Contains(t, context.warnings[1].Message, "missing key \"/b\" in this snippet file, but defined in the main language")
}

func TestSnippetValidateFindsInvalidJsonInMainFile(t *testing.T) {
	tmpDir := t.TempDir()

	context := newValidationContext(PlatformPlugin{
		path:   tmpDir,
		config: &Config{},
	})

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1",}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"a": "2"}`), os.ModePerm)

	assert.NoError(t, validateStorefrontSnippetsByPath(tmpDir, tmpDir, context))
	assert.Len(t, context.errors, 1)
	assert.Len(t, context.warnings, 0)
	assert.Contains(t, context.errors[0].Message, "contains invalid JSON")
}

func TestSnippetValidateFindsInvalidJsonInGermanFile(t *testing.T) {
	tmpDir := t.TempDir()

	context := newValidationContext(PlatformPlugin{
		path:   tmpDir,
		config: &Config{},
	})

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"a": "2",}`), os.ModePerm)

	assert.NoError(t, validateStorefrontSnippetsByPath(tmpDir, tmpDir, context))
	assert.Len(t, context.errors, 1)
	assert.Len(t, context.warnings, 0)
	assert.Contains(t, context.errors[0].Message, "contains invalid JSON")
}
