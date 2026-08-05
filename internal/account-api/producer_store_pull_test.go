package account_api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

func testStoreImage(t *testing.T, priority int, deActivated, enActivated bool, link string) *ExtensionImage {
	t.Helper()

	var image ExtensionImage
	payload := fmt.Sprintf(
		`{"remoteLink":%q,"priority":%d,"details":[{"activated":%t,"locale":{"name":"de_DE"}},{"activated":%t,"locale":{"name":"en_GB"}}]}`,
		link, priority, deActivated, enActivated,
	)
	require.NoError(t, json.Unmarshal([]byte(payload), &image))

	return &image
}

func TestDownloadFileTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("icon-content"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "icon.png")
	require.NoError(t, downloadFileTo(t.Context(), server.Client(), server.URL, target))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "icon-content", string(content))
}

func TestDownloadFileToRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("error page"))
	}))
	defer server.Close()

	target := filepath.Join(t.TempDir(), "icon.png")
	err := downloadFileTo(t.Context(), server.Client(), server.URL, target)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 404")

	_, statErr := os.Stat(target)
	assert.True(t, os.IsNotExist(statErr))
}

func TestWriteImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("img:" + r.URL.Path))
	}))
	defer server.Close()

	// two images share priority 1 in german, so the second one must be moved to priority 2
	images := []*ExtensionImage{
		testStoreImage(t, 1, true, false, server.URL+"/a.png"),
		testStoreImage(t, 1, true, true, server.URL+"/b.png"),
	}

	imageDir := t.TempDir()
	require.NoError(t, writeImages(t.Context(), server.Client(), imageDir, 0, images))
	require.NoError(t, writeImages(t.Context(), server.Client(), imageDir, 1, images))

	deEntries, err := os.ReadDir(filepath.Join(imageDir, "de"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1.png", "2.png"}, fileNames(deEntries))

	enEntries, err := os.ReadDir(filepath.Join(imageDir, "en"))
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"1.png"}, fileNames(enEntries))

	content, err := os.ReadFile(filepath.Join(imageDir, "de", "1.png"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "img:")
}

func TestWriteImagesSkipsInactive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	images := []*ExtensionImage{
		testStoreImage(t, 5, false, false, server.URL+"/a.png"),
	}

	imageDir := t.TempDir()
	require.NoError(t, writeImages(t.Context(), server.Client(), imageDir, 0, images))

	deEntries, err := os.ReadDir(filepath.Join(imageDir, "de"))
	require.NoError(t, err)
	assert.Empty(t, deEntries)
}

func TestWriteImagesSkipsMalformedDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("content"))
	}))
	defer server.Close()

	var image ExtensionImage
	require.NoError(t, json.Unmarshal([]byte(`{"remoteLink":"`+server.URL+`/a.png","priority":1,"details":[]}`), &image))

	imageDir := t.TempDir()
	require.NoError(t, writeImages(t.Context(), server.Client(), imageDir, 0, []*ExtensionImage{&image}))

	deEntries, err := os.ReadDir(filepath.Join(imageDir, "de"))
	require.NoError(t, err)
	assert.Empty(t, deEntries)
}

func fileNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestCollectStoreLanguageInfo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src/Resources/store"), 0o755))

	lang := newStoreLanguageInfo("de")
	assert.False(t, lang.hasContent())

	info := &ExtensionInfo{
		Name:               "Label DE",
		ShortDescription:   "Short DE",
		Description:        "<p>Description</p>",
		InstallationManual: "<p>Install</p>",
		MetaTitle:          "Meta",
		MetaDescription:    "Meta desc",
		Highlights:         "h1\nh2",
		Features:           "f1\nf2",
		Tags:               []StoreTag{{Name: "tag1"}},
		Videos:             []StoreVideo{{URL: "https://video"}},
		Faqs:               []StoreFaq{{Question: "Q?", Answer: "A!", Position: 1}},
	}

	require.NoError(t, collectStoreLanguageInfo(dir, lang, info))
	assert.True(t, lang.hasContent())
	assert.Equal(t, "Label DE", lang.label)
	assert.Equal(t, []string{"tag1"}, lang.tags)
	assert.Equal(t, []string{"https://video"}, lang.videos)
	assert.Equal(t, []string{"h1", "h2"}, lang.highlights)
	assert.Equal(t, []string{"f1", "f2"}, lang.features)
	require.Len(t, lang.faqs, 1)

	desc, err := os.ReadFile(filepath.Join(dir, "src/Resources/store/description.de.html"))
	require.NoError(t, err)
	assert.Equal(t, "<p>Description</p>", string(desc))

	manual, err := os.ReadFile(filepath.Join(dir, "src/Resources/store/installation_manual.de.html"))
	require.NoError(t, err)
	assert.Equal(t, "<p>Install</p>", string(manual))
}

func TestTranslatedFrom(t *testing.T) {
	de := "german"
	en := "english"
	got := translatedFrom(&de, &en)
	assert.Equal(t, &de, got.German)
	assert.Equal(t, &en, got.English)
}

func TestPullExtensionStoreInfo(t *testing.T) {
	assetSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("asset:" + r.URL.Path))
	}))
	defer assetSrv.Close()

	extDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(extDir, "src/Resources"), 0o755))

	var storeExt Extension
	storeExt.Id = 42
	storeExt.Name = "TestPlugin"
	storeExt.IconURL = assetSrv.URL + "/icon.png"
	storeExt.StandardLocale = Locale{Name: "en_GB"}
	storeExt.AutomaticBugfixVersionCompatibility = true
	storeExt.ProductType = &StoreProductType{Name: "extension"}
	storeExt.Localizations = []Locale{{Name: "de_DE"}, {Name: "en_GB"}}
	storeExt.StoreAvailabilities = []StoreAvailablity{{Name: "global"}}
	storeExt.Demos = []ExtensionDemo{{
		Type:          StoreDemoType{Name: "frontend"},
		Link:          "https://demo.example",
		Localization:  Locale{Name: "de_DE"},
		LoginName:     "demo",
		LoginPassword: "secret",
	}}
	storeExt.Infos = []*ExtensionInfo{
		{
			Locale:             Locale{Name: "de_DE"},
			Name:               "DE Label",
			ShortDescription:   "DE Short",
			Description:        "DE Description",
			InstallationManual: "DE Manual",
			MetaTitle:          "DE Meta",
			Tags:               []StoreTag{{Name: "zahlung"}},
		},
		{
			Locale:             Locale{Name: "en_GB"},
			Name:               "EN Label",
			ShortDescription:   "EN Short",
			Description:        "EN Description",
			InstallationManual: "EN Manual",
			MetaTitle:          "EN Meta",
		},
	}

	producer := &fakeProducer{
		getExtensionByNameFn: func(_ context.Context, name string) (*Extension, error) {
			assert.Equal(t, "TestPlugin", name)
			return &storeExt, nil
		},
		getExtensionImagesFn: func(_ context.Context, extensionId int) ([]*ExtensionImage, error) {
			assert.Equal(t, 42, extensionId)
			return []*ExtensionImage{
				testStoreImage(t, 1, true, true, assetSrv.URL+"/img.png"),
			}, nil
		},
	}

	zipExt := &fakeExtension{
		name:   "TestPlugin",
		path:   extDir,
		config: &extension.Config{FileName: ".shopware-extension.yml"},
	}

	require.NoError(t, PullExtensionStoreInfo(t.Context(), producer, zipExt, PullOptions{HTTPClient: assetSrv.Client()}))

	// assets downloaded
	icon, err := os.ReadFile(filepath.Join(extDir, "src/Resources/store/icon.png"))
	require.NoError(t, err)
	assert.Contains(t, string(icon), "asset:")

	// config written
	cfgContent, err := os.ReadFile(filepath.Join(extDir, ".shopware-extension.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(cfgContent), "icon.png")
	assert.Contains(t, string(cfgContent), "global")

	// metadata updated
	require.NotNil(t, zipExt.updated)
	assert.Equal(t, "DE Label", zipExt.updated.Label.German)
	assert.Equal(t, "EN Label", zipExt.updated.Label.English)

	// description files written
	_, err = os.Stat(filepath.Join(extDir, "src/Resources/store/description.de.html"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(extDir, "src/Resources/store/images/de/1.png"))
	require.NoError(t, err)
}

func TestPullExtensionStoreInfoGetNameError(t *testing.T) {
	err := PullExtensionStoreInfo(t.Context(), &fakeProducer{}, &errorNameExtension{}, PullOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot get extension name")
}

type errorNameExtension struct {
	fakeExtension
}

func (e *errorNameExtension) GetName() (string, error) {
	return "", fmt.Errorf("name missing")
}
