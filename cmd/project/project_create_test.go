package project

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
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

// serveShopwareCoreVersions points the create flow at a local repository server
// that knows shopware/core in the given versions, and returns the number of
// requests that server has answered so far.
func serveShopwareCoreVersions(t *testing.T, versions ...string) func() int {
	t.Helper()

	pkg := &repository.Package{Name: "shopware/core"}
	for _, v := range versions {
		pkg.Versions = append(pkg.Versions, repository.Version{Name: pkg.Name, Version: v})
	}

	var requests atomic.Int64
	handler := repository.NewHandler(fakeAdvisoryProvider{pkg: pkg})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	oldURL := packagistURL
	packagistURL = srv.URL
	t.Cleanup(func() { packagistURL = oldURL })

	return func() int { return int(requests.Load()) }
}

// withoutInteraction puts a non-interactive context on the package-level create
// command and restores the previous one afterwards.
func withoutInteraction(t *testing.T) {
	t.Helper()

	oldCtx := projectCreateCmd.Context()
	t.Cleanup(func() { projectCreateCmd.SetContext(oldCtx) })
	projectCreateCmd.SetContext(system.WithInteraction(context.Background(), false))
}

func TestProjectCreateCompletesVersions(t *testing.T) {
	requests := serveShopwareCoreVersions(t, "6.6.5.0", "6.6.10.0", "6.3.0.0", "dev-trunk")
	withoutInteraction(t)

	require.NotNil(t, projectCreateCmd.ValidArgsFunction)

	t.Run("the project name completes directories without asking the repository", func(t *testing.T) {
		before := requests()

		completions, directive := projectCreateCmd.ValidArgsFunction(projectCreateCmd, []string{}, "")

		assert.Empty(t, completions)
		assert.Equal(t, cobra.ShellCompDirectiveFilterDirs, directive)
		assert.Equal(t, before, requests())
	})

	t.Run("the version completes the installable releases", func(t *testing.T) {
		completions, directive := projectCreateCmd.ValidArgsFunction(projectCreateCmd, []string{"some-folder"}, "")

		// Newest first, dev branches and releases below 6.4.18.0 dropped.
		assert.Equal(t, []string{shop.VersionLatest, "6.6.10.0", "6.6.5.0"}, completions)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("a third argument completes nothing", func(t *testing.T) {
		completions, directive := projectCreateCmd.ValidArgsFunction(projectCreateCmd, []string{"some-folder", "latest"}, "")

		assert.Empty(t, completions)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}

// A version the repository does not serve fails on the resolved release list,
// before the project folder is touched and before PHP or Composer are needed.
func TestProjectCreateRejectsUnknownVersion(t *testing.T) {
	serveShopwareCoreVersions(t, "6.6.5.0", "6.6.10.0")
	withoutInteraction(t)

	projectFolder := t.TempDir()

	err := projectCreateCmd.RunE(projectCreateCmd, []string{projectFolder, "9.9.9"})

	assert.ErrorContains(t, err, "cannot find version 9.9.9")
	assert.Empty(t, listDirEntries(t, projectFolder))
}

func listDirEntries(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
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
