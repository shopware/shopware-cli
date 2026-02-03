package extension

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSnippetValidateNoExistingFolderAdmin(t *testing.T) {
	check := &testCheck{}
	plugin := PlatformPlugin{
		path:   "test",
		config: &Config{},
	}

	validateAdministrationSnippets(plugin, check)
}

func TestSnippetValidateNoExistingFolderStorefront(t *testing.T) {
	check := &testCheck{}
	plugin := PlatformPlugin{
		path:   "test",
		config: &Config{},
	}

	validateAdministrationSnippets(plugin, check)
}

func TestSnippetValidateStorefrontByPathOneFileIsIgnored(t *testing.T) {
	tmpDir := t.TempDir()

	check := &testCheck{}
	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{}`), os.ModePerm)

	assert.NoError(t, validateSnippetsByPath(tmpDir, tmpDir, check, storefrontSnippetFilter))
	assert.Len(t, check.Results, 0)
	assert.Len(t, check.Results, 0)
}

func TestSnippetValidateStorefrontByPathSameFile(t *testing.T) {
	tmpDir := t.TempDir()

	check := &testCheck{}

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"test": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"test": "2"}`), os.ModePerm)

	assert.NoError(t, validateSnippetsByPath(tmpDir, tmpDir, check, storefrontSnippetFilter))
	assert.Len(t, check.Results, 0)
	assert.Len(t, check.Results, 0)
}

func TestSnippetValidateStorefrontByPathTestDifferent(t *testing.T) {
	tmpDir := t.TempDir()

	check := &testCheck{}

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"b": "2"}`), os.ModePerm)

	assert.NoError(t, validateSnippetsByPath(tmpDir, tmpDir, check, storefrontSnippetFilter))
	assert.Len(t, check.Results, 2)
	assert.Contains(t, check.Results[0].Message, "key /a is missing, but defined in the main language file")
	assert.Contains(t, check.Results[1].Message, "missing key \"/b\" in this snippet file, but defined in the main language")
}

func TestSnippetValidateFindsInvalidJsonInMainFile(t *testing.T) {
	tmpDir := t.TempDir()

	check := &testCheck{}

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1",}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"a": "2"}`), os.ModePerm)

	assert.NoError(t, validateSnippetsByPath(tmpDir, tmpDir, check, storefrontSnippetFilter))
	assert.Len(t, check.Results, 1)
	assert.Contains(t, check.Results[0].Message, "contains invalid JSON")
}

func TestSnippetValidateFindsInvalidJsonInGermanFile(t *testing.T) {
	tmpDir := t.TempDir()

	check := &testCheck{}

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"a": "2",}`), os.ModePerm)

	assert.NoError(t, validateSnippetsByPath(tmpDir, tmpDir, check, storefrontSnippetFilter))
	assert.Len(t, check.Results, 1)
	assert.Contains(t, check.Results[0].Message, "contains invalid JSON")
}

func TestSnippetValidatePrioritizesCountryAgnosticEnglish(t *testing.T) {
	tmpDir := t.TempDir()
	check := &testCheck{}

	_ = os.MkdirAll(path.Join(tmpDir, "Resources", "snippet"), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en.json"), []byte(`{"a": "1"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.en-GB.json"), []byte(`{"a": "1", "b": "2"}`), os.ModePerm)
	_ = os.WriteFile(path.Join(tmpDir, "Resources", "snippet", "storefront.de-DE.json"), []byte(`{"a": "3"}`), os.ModePerm)

	assert.NoError(t, validateSnippetsByPath(tmpDir, tmpDir, check, storefrontSnippetFilter))
	assert.Len(t, check.Results, 1)
	assert.Contains(
		t,
		check.Results[0].Message,
		"en-GB.json, missing key \"/b\" in this snippet file, but defined in the main language",
	)
}
