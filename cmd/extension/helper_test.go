package extension

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/verifier"
)

const testPluginComposerJSON = `{
	"name": "frosh/frosh-test",
	"type": "shopware-platform-plugin",
	"license": "MIT",
	"version": "1.0.0",
	"require": { "shopware/core": "~6.6.0" },
	"autoload": { "psr-4": { "FroshTest\\": "src/" } },
	"extra": {
		"shopware-plugin-class": "FroshTest\\FroshTest",
		"label": { "de-DE": "Test", "en-GB": "Test" }
	}
}`

// writePluginFixture creates a minimal platform plugin folder.
func writePluginFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(testPluginComposerJSON), 0o644))
	return dir
}

// buildExtensionZip zips the fixture under a top-level FroshTest/ folder, the
// layout GetExtensionByZip derives the extension directory from.
func buildExtensionZip(t *testing.T, dir string) string {
	t.Helper()

	zipPath := filepath.Join(t.TempDir(), "FroshTest.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)

	require.NoError(t, filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		entry, err := w.Create("FroshTest/" + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(entry, src)
		if closeErr := src.Close(); err == nil {
			err = closeErr
		}
		return err
	}))
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())

	return zipPath
}

// fakeShopwareVersions makes verifier.ConvertExtensionToToolConfig work
// offline; it would otherwise fetch shopware/core versions from Packagist.
func fakeShopwareVersions(t *testing.T) {
	t.Helper()
	restore := verifier.OverrideShopwareVersionsForTesting(func(context.Context) ([]string, error) {
		return []string{"6.6.0.0", "6.6.10.0"}, nil
	})
	t.Cleanup(restore)
}

// resetFlagsNow restores all of the command's flags to their defaults; flag
// values and Changed state persist across Execute calls.
func resetFlagsNow(cmd *cobra.Command) {
	reset := func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	}
	cmd.PersistentFlags().VisitAll(reset)
	cmd.Flags().VisitAll(reset)
}

// resetCommandFlags restores the command's flags to their defaults in cleanup.
func resetCommandFlags(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	t.Cleanup(func() { resetFlagsNow(cmd) })
}

// setContextRecursively sets ctx on the command and all of its subcommands;
// cobra only inherits the parent context into a subcommand whose own context is
// still nil, so a subcommand would otherwise keep the context of the first test
// that executed it.
func setContextRecursively(cmd *cobra.Command, ctx context.Context) {
	cmd.SetContext(ctx)
	for _, sub := range cmd.Commands() {
		setContextRecursively(sub, ctx)
	}
}

// runExtensionCtx executes the extension root command with the given context.
func runExtensionCtx(t *testing.T, ctx context.Context, args ...string) error {
	t.Helper()
	setContextRecursively(extensionRootCmd, ctx)
	extensionRootCmd.SetArgs(args)
	t.Cleanup(func() {
		extensionRootCmd.SetArgs(nil)
		setContextRecursively(extensionRootCmd, context.Background())
	})
	return extensionRootCmd.Execute()
}

func runExtension(t *testing.T, args ...string) error {
	t.Helper()
	return runExtensionCtx(t, t.Context(), args...)
}

// captureStdout captures os.Stdout while fn runs; several commands print via
// fmt.Println instead of the cobra out writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	require.NoError(t, w.Close())
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}
