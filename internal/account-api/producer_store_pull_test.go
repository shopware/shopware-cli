package account_api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
