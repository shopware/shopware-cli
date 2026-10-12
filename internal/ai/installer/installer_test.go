package installer

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	for _, tc := range []struct {
		action  string
		options Options
		args    []string
	}{
		{"add", Options{}, []string{"npx", "--yes", "skills", "add", "https://github.com/shopware/shopware-cli", "--skill", "shopware-cli", "--yes"}},
		{"remove", Options{}, []string{"npx", "--yes", "skills", "remove", "shopware-cli", "--yes"}},
		{"add", Options{Client: "claude", Global: true}, []string{"npx", "--yes", "skills", "add", "https://github.com/shopware/shopware-cli", "--skill", "shopware-cli", "--agent", "claude", "--yes", "--global"}},
	} {
		t.Run(tc.action+tc.options.Client, func(t *testing.T) {
			called := false
			result, err := run(t.Context(), tc.action, "shopware-cli", tc.options, io.Discard, func(_ context.Context, cwd string, args []string, stdout, stderr io.Writer) error {
				called = true
				assert.DirExists(t, cwd)
				assert.Equal(t, tc.args, args)
				assert.Equal(t, io.Discard, stdout)
				assert.Equal(t, io.Discard, stderr)
				return nil
			})
			require.NoError(t, err)
			assert.True(t, called)
			assert.Equal(t, tc.args, result.Command)
		})
	}
}

func TestDryRun(t *testing.T) {
	for _, action := range []string{"add", "remove"} {
		result, err := run(t.Context(), action, "shopware-cli", Options{DryRun: true}, io.Discard, unexpectedRunner(t))
		require.NoError(t, err)
		assert.True(t, result.DryRun)
	}
}

func unexpectedRunner(t *testing.T) runner {
	t.Helper()
	return func(context.Context, string, []string, io.Writer, io.Writer) error {
		t.Fatal("unexpected npx invocation")
		return nil
	}
}

func TestInvalidRequestsDoNotRun(t *testing.T) {
	for _, tc := range []struct{ action, name, client string }{
		{"add", "unknown", "codex"},
		{"remove", "unknown", "codex"},
		{"update", "shopware-cli", "codex"},
	} {
		_, err := run(t.Context(), tc.action, tc.name, Options{Client: tc.client}, io.Discard, unexpectedRunner(t))
		require.Error(t, err)
	}
}

func TestSubprocessFailure(t *testing.T) {
	failure := errors.New("subprocess failed")
	for _, action := range []string{"add", "remove"} {
		_, err := run(t.Context(), action, "shopware-cli", Options{}, io.Discard, func(context.Context, string, []string, io.Writer, io.Writer) error { return failure })
		require.ErrorIs(t, err, failure)
	}
}

func TestExecute(t *testing.T) {
	err := execute(t.Context(), t.TempDir(), []string{"shopware-cli-nonexistent-executable"}, io.Discard, io.Discard)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err = execute(ctx, t.TempDir(), []string{"go", "version"}, io.Discard, io.Discard)
	require.Error(t, err)
}

func TestInstalledNames(t *testing.T) {
	for _, global := range []bool{false, true} {
		names, err := installedNames(t.Context(), global, io.Discard, func(_ context.Context, cwd string, args []string, stdout, stderr io.Writer) error {
			expected := []string{"npx", "--yes", "skills", "list", "--json"}
			if global {
				expected = append(expected, "--global")
			}
			assert.Equal(t, expected, args)
			assert.Empty(t, cwd)
			assert.Equal(t, io.Discard, stderr)
			_, err := io.WriteString(stdout, `[{"name":"shopware-cli","scope":"project","agents":["Codex"],"source":null},{"name":"other-skill"}]`)
			return err
		})
		require.NoError(t, err)
		assert.Equal(t, map[string]bool{"shopware-cli": true, "other-skill": true}, names)
	}
}

func TestInstalledNamesInvalidJSON(t *testing.T) {
	for _, output := range []string{"not json", `{}`, `[{"name":42}]`} {
		_, err := installedNames(t.Context(), false, io.Discard, func(_ context.Context, _ string, _ []string, stdout, _ io.Writer) error {
			_, err := io.WriteString(stdout, output)
			return err
		})
		require.Error(t, err)
	}
}

func TestInstalledNamesEmptyAndFailure(t *testing.T) {
	names, err := installedNames(t.Context(), false, io.Discard, func(_ context.Context, _ string, _ []string, stdout, _ io.Writer) error {
		_, err := io.WriteString(stdout, "[]")
		return err
	})
	require.NoError(t, err)
	assert.Empty(t, names)
	failure := errors.New("npx failed")
	_, err = installedNames(t.Context(), false, io.Discard, func(context.Context, string, []string, io.Writer, io.Writer) error { return failure })
	require.ErrorIs(t, err, failure)
}
