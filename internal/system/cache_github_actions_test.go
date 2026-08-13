package system

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	actionscache "github.com/tonistiigi/go-actions-cache"
)

// fakeActionsCacheClient records Save calls and returns a canned error.
type fakeActionsCacheClient struct {
	saveErr   error
	savedKeys []string
}

// Load is only present to satisfy actionsCacheClient; these tests exercise the save paths.
func (f *fakeActionsCacheClient) Load(ctx context.Context, keys ...string) (*actionscache.Entry, error) {
	return nil, ErrCacheNotFound
}

func (f *fakeActionsCacheClient) Save(ctx context.Context, key string, b actionscache.Blob) error {
	f.savedKeys = append(f.savedKeys, key)
	return f.saveErr
}

// alreadyExistsError mimics what the GitHub Actions cache API returns when an
// entry for the key was already uploaded, wrapped the same way the library does.
func alreadyExistsError() error {
	return actionscache.HTTPError{
		StatusCode: 409,
		Err: actionscache.GithubAPIError{
			Message: "409 Conflict",
			TypeKey: "ArtifactCacheItemAlreadyExistsException",
		},
	}
}

func TestGitHubActionsCacheSetIgnoresAlreadyExists(t *testing.T) {
	t.Parallel()

	client := &fakeActionsCacheClient{saveErr: alreadyExistsError()}
	cache := &GitHubActionsCache{client: client, prefix: "sw-cli"}

	err := cache.Set(t.Context(), "some-key", strings.NewReader("content"))

	require.NoError(t, err, "already existing cache entry must not fail the build")
	assert.Len(t, client.savedKeys, 1)
}

func TestGitHubActionsCacheSetReturnsOtherErrors(t *testing.T) {
	t.Parallel()

	client := &fakeActionsCacheClient{saveErr: assert.AnError}
	cache := &GitHubActionsCache{client: client, prefix: "sw-cli"}

	err := cache.Set(t.Context(), "some-key", strings.NewReader("content"))

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestGitHubActionsCacheStoreFolderCacheIgnoresAlreadyExists(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(folder, "app.js"), []byte("console.log(1)"), 0o644))

	client := &fakeActionsCacheClient{saveErr: alreadyExistsError()}
	cache := &GitHubActionsCache{client: client, prefix: "sw-cli"}

	err := cache.StoreFolderCache(t.Context(), "sw-cli-6.7.0.0-abc", folder)

	require.NoError(t, err, "already existing folder cache entry must not fail the build")
	assert.Len(t, client.savedKeys, 1)
}

func TestGitHubActionsCacheStoreFolderCacheReturnsOtherErrors(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(folder, "app.js"), []byte("console.log(1)"), 0o644))

	client := &fakeActionsCacheClient{saveErr: assert.AnError}
	cache := &GitHubActionsCache{client: client, prefix: "sw-cli"}

	err := cache.StoreFolderCache(t.Context(), "sw-cli-6.7.0.0-abc", folder)

	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}
