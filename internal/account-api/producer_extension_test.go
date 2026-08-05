package account_api

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExtensionBinaries(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/producers/1/plugins/2/binaries", r.URL.Path)
		_ = json.NewEncoder(w).Encode([]*ExtensionBinary{{Id: 3, Version: "1.0.0"}})
	})

	endpoint := &ProducerEndpoint{c: client}
	binaries, err := endpoint.GetExtensionBinaries(t.Context(), 1, 2)
	require.NoError(t, err)
	require.Len(t, binaries, 1)
	assert.Equal(t, "1.0.0", binaries[0].Version)
}

func TestUpdateExtensionBinaryInfo(t *testing.T) {
	var body ExtensionUpdate
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/producers/1/plugins/2/binaries/3", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		w.WriteHeader(http.StatusOK)
	})

	endpoint := &ProducerEndpoint{c: client}
	require.NoError(t, endpoint.UpdateExtensionBinaryInfo(t.Context(), 1, 2, ExtensionUpdate{
		Id:               3,
		SoftwareVersions: []string{"6.5.0.0"},
	}))
	assert.Equal(t, []string{"6.5.0.0"}, body.SoftwareVersions)
}

func TestCreateExtensionBinary(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/producers/1/plugins/2/binaries", r.URL.Path)
		_ = json.NewEncoder(w).Encode(&ExtensionBinary{Id: 9, Version: "2.0.0"})
	})

	endpoint := &ProducerEndpoint{c: client}
	binary, err := endpoint.CreateExtensionBinary(t.Context(), 1, 2, ExtensionCreate{Version: "2.0.0"})
	require.NoError(t, err)
	assert.Equal(t, 9, binary.Id)
	assert.Equal(t, "2.0.0", binary.Version)
}

func TestUpdateExtensionBinaryFile(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "ext.zip")
	require.NoError(t, os.WriteFile(zipPath, []byte("zip-content"), 0o644))

	var contentType string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/producers/1/plugins/2/binaries/3/file", r.URL.Path)
		contentType = r.Header.Get("content-type")
		w.WriteHeader(http.StatusOK)
	})

	endpoint := &ProducerEndpoint{c: client}
	require.NoError(t, endpoint.UpdateExtensionBinaryFile(t.Context(), 1, 2, 3, zipPath))
	assert.Contains(t, contentType, "multipart/form-data")
}

func TestUpdateExtensionIconResizes(t *testing.T) {
	iconPath := filepath.Join(t.TempDir(), "icon.png")
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	f, err := os.Create(iconPath)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/plugins/7/icon", r.URL.Path)
		assert.Contains(t, r.Header.Get("content-type"), "multipart/form-data")
		w.WriteHeader(http.StatusOK)
	})

	endpoint := &ProducerEndpoint{c: client}
	require.NoError(t, endpoint.UpdateExtensionIcon(t.Context(), 7, iconPath))
}

func TestUpdateExtensionIconAlready256(t *testing.T) {
	iconPath := filepath.Join(t.TempDir(), "icon.png")
	img := image.NewRGBA(image.Rect(0, 0, 256, 256))
	f, err := os.Create(iconPath)
	require.NoError(t, err)
	require.NoError(t, png.Encode(f, img))
	require.NoError(t, f.Close())

	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/plugins/7/icon", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	endpoint := &ProducerEndpoint{c: client}
	require.NoError(t, endpoint.UpdateExtensionIcon(t.Context(), 7, iconPath))
}

func TestExtensionImageCRUD(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/plugins/1/pictures":
			_ = json.NewEncoder(w).Encode([]*ExtensionImage{{Id: 11, RemoteLink: "https://img"}})
		case r.Method == http.MethodDelete && r.URL.Path == "/plugins/1/pictures/11":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/plugins/1/pictures/11":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/plugins/1/pictures":
			_ = json.NewEncoder(w).Encode([]*ExtensionImage{{Id: 12, RemoteLink: "https://new"}})
		default:
			http.NotFound(w, r)
		}
	})

	endpoint := &ProducerEndpoint{c: client}

	images, err := endpoint.GetExtensionImages(t.Context(), 1)
	require.NoError(t, err)
	require.Len(t, images, 1)
	assert.Equal(t, 11, images[0].Id)

	require.NoError(t, endpoint.DeleteExtensionImages(t.Context(), 1, 11))
	require.NoError(t, endpoint.UpdateExtensionImage(t.Context(), 1, &ExtensionImage{Id: 11}))

	filePath := filepath.Join(t.TempDir(), "img.png")
	require.NoError(t, os.WriteFile(filePath, []byte("png"), 0o644))
	created, err := endpoint.AddExtensionImage(t.Context(), 1, filePath)
	require.NoError(t, err)
	assert.Equal(t, 12, created.Id)
}

func TestTriggerCodeReviewAndGetBinaryReviewResults(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/plugins/1/reviews":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/plugins/1/binaries/2/checkresults":
			_ = json.NewEncoder(w).Encode([]BinaryReviewResult{reviewWithType(3)})
		default:
			http.NotFound(w, r)
		}
	})

	endpoint := &ProducerEndpoint{c: client}
	require.NoError(t, endpoint.TriggerCodeReview(t.Context(), 1))

	results, err := endpoint.GetBinaryReviewResults(t.Context(), 1, 2)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, results[0].HasPassed())
}

func TestBinaryReviewHelpers(t *testing.T) {
	passedByName := BinaryReviewResult{}
	passedByName.Type.Name = "automaticcodereviewsucceeded"
	assert.True(t, passedByName.HasPassed())
	assert.False(t, BinaryReviewResult{}.HasPassed())

	withWarning := BinaryReviewResult{}
	withWarning.SubCheckResults = append(withWarning.SubCheckResults, struct {
		SubCheck    string `json:"subCheck"`
		Status      string `json:"status"`
		Passed      bool   `json:"passed"`
		Message     string `json:"message"`
		HasWarnings bool   `json:"hasWarnings"`
	}{SubCheck: "php", Passed: true, HasWarnings: true, Message: "warn"})
	assert.True(t, withWarning.HasWarnings())
	assert.False(t, BinaryReviewResult{}.HasWarnings())

	summary := withWarning.GetSummary()
	assert.Contains(t, summary, "php")
	assert.Contains(t, summary, "warn")

	// passed without warnings is omitted from summary
	clean := BinaryReviewResult{}
	clean.SubCheckResults = append(clean.SubCheckResults, struct {
		SubCheck    string `json:"subCheck"`
		Status      string `json:"status"`
		Passed      bool   `json:"passed"`
		Message     string `json:"message"`
		HasWarnings bool   `json:"hasWarnings"`
	}{SubCheck: "ok", Passed: true, HasWarnings: false, Message: "fine"})
	assert.Empty(t, clean.GetSummary())
}

func TestFilterOnVersion(t *testing.T) {
	list := SoftwareVersionList{
		{Name: "6.4.0.0", Selectable: true},
		{Name: "6.5.0.0", Selectable: true},
		{Name: "6.6.0.0", Selectable: false},
		{Name: "not-a-version", Selectable: true},
	}

	constraint, err := version.NewConstraint("~6.5.0")
	require.NoError(t, err)

	filtered := list.FilterOnVersion(&constraint)
	require.Len(t, filtered, 1)
	assert.Equal(t, "6.5.0.0", filtered[0].Name)

	names := list.FilterOnVersionStringList(&constraint)
	assert.Equal(t, []string{"6.5.0.0"}, names)
}

func TestUpdateExtensionBinaryFileMissingZip(t *testing.T) {
	endpoint := &ProducerEndpoint{c: &Client{}}
	err := endpoint.UpdateExtensionBinaryFile(t.Context(), 1, 2, 3, filepath.Join(t.TempDir(), "missing.zip"))
	require.Error(t, err)
}
