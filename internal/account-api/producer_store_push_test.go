package account_api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

func demoGeneralInfo() *ExtensionGeneralInformation {
	return &ExtensionGeneralInformation{
		Locales: []Locale{
			{Id: 1, Name: "de_DE"},
			{Id: 2, Name: "en_GB"},
		},
		DemoTypes: []StoreDemoType{
			{Id: 1, Name: "frontend", Description: "Frontend demo"},
			{Id: 2, Name: "backend", Description: "Backend demo"},
		},
	}
}

func TestBuildExtensionDemos(t *testing.T) {
	demos, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{
		{
			Type:          "frontend",
			Link:          "https://demo.example.com",
			Localization:  "de_DE",
			LoginName:     "demo",
			LoginPassword: "secret",
		},
		{
			Type:         "backend",
			Link:         "https://demo.example.com/admin",
			Localization: "en_GB",
		},
	}, demoGeneralInfo())

	require.NoError(t, err)
	assert.Equal(t, []ExtensionDemo{
		{
			Type:          StoreDemoType{Id: 1, Name: "frontend", Description: "Frontend demo"},
			Link:          "https://demo.example.com",
			Localization:  Locale{Id: 1, Name: "de_DE"},
			LoginName:     "demo",
			LoginPassword: "secret",
		},
		{
			Type:         StoreDemoType{Id: 2, Name: "backend", Description: "Backend demo"},
			Link:         "https://demo.example.com/admin",
			Localization: Locale{Id: 2, Name: "en_GB"},
		},
	}, demos)
}

func TestBuildExtensionDemosEmptyList(t *testing.T) {
	demos, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{}, demoGeneralInfo())

	require.NoError(t, err)
	assert.Empty(t, demos)
	assert.NotNil(t, demos)
}

func TestBuildExtensionDemosUnknownType(t *testing.T) {
	_, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{
		{Type: "storefront", Link: "https://demo.example.com", Localization: "de_DE"},
	}, demoGeneralInfo())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown demo shop type \"storefront\"")
	assert.Contains(t, err.Error(), "frontend, backend")
}

func TestBuildExtensionDemosUnknownLocalization(t *testing.T) {
	_, err := buildExtensionDemos([]extension.ConfigStoreDemoShop{
		{Type: "frontend", Link: "https://demo.example.com", Localization: "fr_FR"},
	}, demoGeneralInfo())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown demo shop localization \"fr_FR\"")
	assert.Contains(t, err.Error(), "de_DE, en_GB")
}

func TestUpdateStoreInfoKeepsDemosWhenNotConfigured(t *testing.T) {
	ext := &Extension{
		Demos: []ExtensionDemo{
			{Id: 42, Type: StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://demo.example.com"},
		},
	}

	require.NoError(t, updateStoreInfo(ext, nil, &extension.Config{}, demoGeneralInfo()))
	assert.Len(t, ext.Demos, 1)
	assert.Equal(t, 42, ext.Demos[0].Id)
}

func TestUpdateStoreInfoReplacesDemos(t *testing.T) {
	ext := &Extension{
		Demos: []ExtensionDemo{
			{Id: 42, Type: StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://old.example.com"},
		},
	}

	cfg := &extension.Config{}
	cfg.Store.DemoShops = &[]extension.ConfigStoreDemoShop{
		{Type: "backend", Link: "https://demo.example.com/admin", Localization: "en_GB"},
	}

	require.NoError(t, updateStoreInfo(ext, nil, cfg, demoGeneralInfo()))
	require.Len(t, ext.Demos, 1)
	assert.Equal(t, 0, ext.Demos[0].Id)
	assert.Equal(t, "backend", ext.Demos[0].Type.Name)
	assert.Equal(t, "https://demo.example.com/admin", ext.Demos[0].Link)
	assert.Equal(t, "en_GB", ext.Demos[0].Localization.Name)
}

func TestUpdateStoreInfoClearsDemos(t *testing.T) {
	ext := &Extension{
		Demos: []ExtensionDemo{
			{Id: 42, Type: StoreDemoType{Id: 1, Name: "frontend"}, Link: "https://demo.example.com"},
		},
	}

	cfg := &extension.Config{}
	cfg.Store.DemoShops = &[]extension.ConfigStoreDemoShop{}

	require.NoError(t, updateStoreInfo(ext, nil, cfg, demoGeneralInfo()))
	assert.Empty(t, ext.Demos)
}

func TestUpdateStoreInfoAppliesTranslations(t *testing.T) {
	var ext Extension
	require.NoError(t, json.Unmarshal([]byte(`{"infos":[{"locale":{"name":"de_DE"}},{"locale":{"name":"en_GB"}}]}`), &ext))

	deMetaTitle := "German title"
	enMetaTitle := "English title"
	deTags := []string{"tag-de"}
	enTags := []string{"tag-en"}

	cfg := &extension.Config{}
	cfg.Store.MetaTitle = extension.ConfigTranslated[string]{German: &deMetaTitle, English: &enMetaTitle}
	cfg.Store.Tags = extension.ConfigTranslated[[]string]{German: &deTags, English: &enTags}

	require.NoError(t, updateStoreInfo(&ext, nil, cfg, demoGeneralInfo()))

	assert.Equal(t, "German title", ext.Infos[0].MetaTitle)
	assert.Equal(t, "English title", ext.Infos[1].MetaTitle)
	assert.Equal(t, []StoreTag{{Name: "tag-de"}}, ext.Infos[0].Tags)
	assert.Equal(t, []StoreTag{{Name: "tag-en"}}, ext.Infos[1].Tags)
}

func TestGetTranslation(t *testing.T) {
	german := "de"
	english := "en"
	config := extension.ConfigTranslated[string]{German: &german, English: &english}

	assert.Equal(t, &german, getTranslation("de", config))
	assert.Equal(t, &english, getTranslation("en", config))
	assert.Nil(t, getTranslation("fr", config))
}

func TestParseInlineablePathPlainText(t *testing.T) {
	content, err := parseInlineablePath("plain text", "/does-not-matter")

	require.NoError(t, err)
	assert.Equal(t, "plain text", content)
}

func TestParseInlineablePathReadsFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.html"), []byte("<p>Hello</p>"), 0o644))

	content, err := parseInlineablePath("file:description.html", dir)

	require.NoError(t, err)
	assert.Equal(t, "<p>Hello</p>", content)
}

func TestParseInlineablePathConvertsMarkdown(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "description.md"), []byte("# Hello"), 0o644))

	content, err := parseInlineablePath("file:description.md", dir)

	require.NoError(t, err)
	assert.Contains(t, content, "Hello")
	assert.NotEqual(t, "# Hello", content)
}

func TestParseInlineablePathMissingFile(t *testing.T) {
	_, err := parseInlineablePath("file:missing.html", t.TempDir())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "error reading file")
}

func TestLanguageFromLocale(t *testing.T) {
	assert.Equal(t, "de", languageFromLocale("de_DE"))
	assert.Equal(t, "en", languageFromLocale("en_GB"))
	assert.Equal(t, "", languageFromLocale(""))
	assert.Equal(t, "", languageFromLocale("d"))
	assert.Equal(t, "", languageFromLocale("fr_FR"))
}

func TestUploadImagesByDirectory(t *testing.T) {
	dir := t.TempDir()
	deDir := filepath.Join(dir, "de")
	require.NoError(t, os.MkdirAll(deDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(deDir, "1-first.png"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(deDir, "2-second.png"), []byte("x"), 0o644))
	// invalid names and directories must not be uploaded
	require.NoError(t, os.WriteFile(filepath.Join(deDir, ".DS_Store"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(deDir, "README"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(deDir, "subdir"), 0o755))

	var uploaded []string
	var updated []*ExtensionImage
	producer := &fakeProducer{
		addExtensionImageFn: func(_ context.Context, _ int, file string) (*ExtensionImage, error) {
			uploaded = append(uploaded, filepath.Base(file))
			return testStoreImage(t, 0, false, false, ""), nil
		},
		updateExtensionImageFn: func(_ context.Context, _ int, image *ExtensionImage) error {
			updated = append(updated, image)
			return nil
		},
	}

	require.NoError(t, uploadImagesByDirectory(t.Context(), producer, 1, dir, 0))

	assert.ElementsMatch(t, []string{"1-first.png", "2-second.png"}, uploaded)
	require.Len(t, updated, 2)
	assert.Equal(t, 1, updated[0].Priority)
	assert.Equal(t, 2, updated[1].Priority)
	assert.True(t, updated[0].Details[0].Activated)
	// preview is attached to the last valid file only
	assert.False(t, updated[0].Details[0].Preview)
	assert.True(t, updated[1].Details[0].Preview)
}

func TestUploadImagesByDirectoryMissingFolder(t *testing.T) {
	require.NoError(t, uploadImagesByDirectory(t.Context(), &fakeProducer{}, 1, t.TempDir(), 0))
}

func TestPushExtensionStoreInfo(t *testing.T) {
	extDir := t.TempDir()
	iconPath := "src/Resources/store/icon.png"
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "src/Resources/store"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, iconPath), []byte("icon"), 0o644))

	imageDir := "src/Resources/store/images"
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, imageDir, "de"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, imageDir, "de", "1-first.png"), []byte("img"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, imageDir, "en"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, imageDir, "en", "1-first.png"), []byte("img"), 0o644))

	var storeExt Extension
	require.NoError(t, json.Unmarshal([]byte(`{"id":7,"infos":[{"locale":{"name":"de_DE"}},{"locale":{"name":"en_GB"}}]}`), &storeExt))

	var updatedExtension *Extension
	var iconUpdated bool
	var deletedImageIds []int
	var addedImages int

	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, name string) (*Extension, error) {
			assert.Equal(t, "TestPlugin", name)
			return &storeExt, nil
		},
		getExtensionGeneralInfoFn: func(_ context.Context) (*ExtensionGeneralInformation, error) {
			return demoGeneralInfo(), nil
		},
		updateExtensionIconFn: func(_ context.Context, extensionId int, path string) error {
			assert.Equal(t, 7, extensionId)
			assert.Contains(t, path, iconPath)
			iconUpdated = true
			return nil
		},
		getExtensionImagesFn: func(_ context.Context, _ int) ([]*ExtensionImage, error) {
			return []*ExtensionImage{{Id: 99}}, nil
		},
		deleteExtensionImagesFn: func(_ context.Context, _, imageId int) error {
			deletedImageIds = append(deletedImageIds, imageId)
			return nil
		},
		addExtensionImageFn: func(_ context.Context, _ int, _ string) (*ExtensionImage, error) {
			addedImages++
			return testStoreImage(t, 0, false, false, ""), nil
		},
		updateExtensionImageFn: func(_ context.Context, _ int, _ *ExtensionImage) error {
			return nil
		},
		updateExtensionFn: func(_ context.Context, extension *Extension) error {
			updatedExtension = extension
			return nil
		},
	}

	cfg := &extension.Config{FileName: ".shopware-extension.yml"}
	cfg.Store.Icon = &iconPath
	cfg.Store.ImageDirectory = &imageDir

	zipExt := &fakeExtension{
		name:   "TestPlugin",
		path:   extDir,
		config: cfg,
		metadata: &extension.ExtensionMetadata{
			Label:       extension.ExtensionTranslated{German: "DE", English: "EN"},
			Description: extension.ExtensionTranslated{German: "DEsc", English: "ENsc"},
		},
	}

	require.NoError(t, PushExtensionStoreInfo(t.Context(), producer, zipExt))

	assert.True(t, iconUpdated)
	assert.Equal(t, []int{99}, deletedImageIds)
	assert.Equal(t, 2, addedImages)
	require.NotNil(t, updatedExtension)
	assert.Equal(t, "DE", updatedExtension.Infos[0].Name)
	assert.Equal(t, "EN", updatedExtension.Infos[1].Name)
}

func TestPushExtensionStoreInfoNameError(t *testing.T) {
	err := PushExtensionStoreInfo(t.Context(), &fakeProducer{}, &errorNameExtension{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot get name")
}

func TestPushExtensionStoreInfoManualImages(t *testing.T) {
	extDir := t.TempDir()
	imgFile := "src/Resources/store/manual.png"
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "src/Resources/store"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, imgFile), []byte("img"), 0o644))

	var storeExt Extension
	require.NoError(t, json.Unmarshal([]byte(`{"id":7,"infos":[{"locale":{"name":"de_DE"}},{"locale":{"name":"en_GB"}}]}`), &storeExt))

	var updated *ExtensionImage
	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, _ string) (*Extension, error) {
			return &storeExt, nil
		},
		getExtensionGeneralInfoFn: func(_ context.Context) (*ExtensionGeneralInformation, error) {
			return demoGeneralInfo(), nil
		},
		getExtensionImagesFn: func(_ context.Context, _ int) ([]*ExtensionImage, error) {
			return nil, nil
		},
		addExtensionImageFn: func(_ context.Context, _ int, file string) (*ExtensionImage, error) {
			assert.Contains(t, file, imgFile)
			return testStoreImage(t, 0, false, false, ""), nil
		},
		updateExtensionImageFn: func(_ context.Context, _ int, image *ExtensionImage) error {
			updated = image
			return nil
		},
		updateExtensionFn: func(_ context.Context, _ *Extension) error { return nil },
	}

	cfg := &extension.Config{}
	cfg.Store.Images = &[]extension.ConfigStoreImage{
		{
			File:     imgFile,
			Activate: extension.ConfigStoreImageActivate{German: true, English: false},
			Preview:  extension.ConfigStoreImagePreview{German: true, English: false},
		},
	}

	require.NoError(t, PushExtensionStoreInfo(t.Context(), producer, &fakeExtension{
		name:     "TestPlugin",
		path:     extDir,
		config:   cfg,
		metadata: &extension.ExtensionMetadata{},
	}))

	require.NotNil(t, updated)
	assert.True(t, updated.Details[0].Activated)
	assert.True(t, updated.Details[0].Preview)
	assert.False(t, updated.Details[1].Activated)
}

func TestUpdateStoreInfoAppliesStoreFields(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "desc.de.html"), []byte("<p>de</p>"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manual.en.html"), []byte("<p>en</p>"), 0o644))

	var ext Extension
	require.NoError(t, json.Unmarshal([]byte(`{"infos":[{"locale":{"name":"de_DE"}},{"locale":{"name":"en_GB"}}]}`), &ext))

	defaultLocale := "de_DE"
	productType := "extension"
	auto := true
	localizations := []string{"de_DE", "en_GB"}
	availabilities := []string{"global"}
	deDesc := "file:desc.de.html"
	enManual := "file:manual.en.html"
	deHighlights := []string{"h1"}
	enFeatures := []string{"f1"}
	deFaqs := []extension.ConfigStoreFaq{{Question: "Q", Answer: "A", Position: 1}}
	deVideos := []string{"https://v"}
	deMetaDesc := "meta-de"

	info := demoGeneralInfo()
	info.StoreAvailabilities = []StoreAvailablity{{Id: 1, Name: "global"}, {Id: 2, Name: "eu"}}
	info.ProductTypes = []StoreProductType{{Id: 3, Name: "extension"}}

	cfg := &extension.Config{}
	cfg.Store.DefaultLocale = &defaultLocale
	cfg.Store.Localizations = &localizations
	cfg.Store.Availabilities = &availabilities
	cfg.Store.Type = &productType
	cfg.Store.AutomaticBugfixVersionCompatibility = &auto
	cfg.Store.Description = extension.ConfigTranslated[string]{German: &deDesc}
	cfg.Store.InstallationManual = extension.ConfigTranslated[string]{English: &enManual}
	cfg.Store.Highlights = extension.ConfigTranslated[[]string]{German: &deHighlights}
	cfg.Store.Features = extension.ConfigTranslated[[]string]{English: &enFeatures}
	cfg.Store.Faq = extension.ConfigTranslated[[]extension.ConfigStoreFaq]{German: &deFaqs}
	cfg.Store.Videos = extension.ConfigTranslated[[]string]{German: &deVideos}
	cfg.Store.MetaDescription = extension.ConfigTranslated[string]{German: &deMetaDesc}

	zipExt := &fakeExtension{path: dir}
	require.NoError(t, updateStoreInfo(&ext, zipExt, cfg, info))

	assert.Equal(t, "de_DE", ext.StandardLocale.Name)
	assert.Equal(t, []Locale{{Name: "de_DE"}, {Name: "en_GB"}}, ext.Localizations)
	require.Len(t, ext.StoreAvailabilities, 1)
	assert.Equal(t, "global", ext.StoreAvailabilities[0].Name)
	require.NotNil(t, ext.ProductType)
	assert.Equal(t, "extension", ext.ProductType.Name)
	assert.True(t, ext.AutomaticBugfixVersionCompatibility)
	assert.Equal(t, "<p>de</p>", ext.Infos[0].Description)
	assert.Equal(t, "<p>en</p>", ext.Infos[1].InstallationManual)
	assert.Equal(t, "h1", ext.Infos[0].Highlights)
	assert.Equal(t, "f1", ext.Infos[1].Features)
	require.Len(t, ext.Infos[0].Faqs, 1)
	assert.Equal(t, []StoreVideo{{URL: "https://v"}}, ext.Infos[0].Videos)
	assert.Equal(t, "meta-de", ext.Infos[0].MetaDescription)
}
