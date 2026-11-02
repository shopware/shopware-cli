package ssh

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHRolloutRejectsUnsafeInputsBeforeSSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	archive := filepath.Join(t.TempDir(), "shop.tar.gz")
	testhelper.WriteFile(t, archive, "archive")
	for _, dir := range []string{"relative/current", "/var/www/releases", "/current"} {
		e := &SSH{directory: dir}
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
		require.ErrorContains(t, err, "ssh.directory")
	}
	e := &SSH{directory: "/var/www/shop/current"}
	for _, reference := range []string{"", "missing", t.TempDir()} {
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: reference}, nil)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "executable file not found", "must fail before looking for ssh")
	}
}

func TestSSHRolloutRejectsInvalidSharedPathsBeforeSSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"../outside", "public"} {
		directories := []string{name}
		e := &SSH{directory: "/var/www/shop/current", env: &shop.EnvironmentConfig{
			SSH: &shop.EnvironmentSSHConfig{Shared: &shop.EnvironmentSSHSharedConfig{Directories: &directories}},
		}}
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: "unused.tar.gz"}, nil)
		require.ErrorContains(t, err, "ssh.shared")
	}
}

func TestDeploymentHandshakeDoesNotAuthorizeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, response := range []string{"CACHED id\nPREPARE id\nREADY id\n", "REUSE id\nREADY id\n"} {
		var stdin bytes.Buffer
		_, err := exchangeSSHDeployment(
			ctx,
			&stdin,
			bufio.NewReader(strings.NewReader(response)),
			bytes.NewReader(nil),
			sshRolloutInput{Reference: "id"},
			ci.New(io.Discard),
		)
		require.ErrorIs(t, err, context.Canceled)
		assert.NotContains(t, stdin.String(), "ACTIVATE")
	}
	require.Error(t, readRolloutMessage(bufio.NewReader(strings.NewReader(strings.Repeat("x", 5000))), "READY"))
	require.ErrorIs(t, readRolloutMessage(bufio.NewReader(strings.NewReader("")), "READY"), io.EOF)
}

func TestDeploymentHandshakeSkipsCachedArchive(t *testing.T) {
	archive := bytes.NewBufferString("must remain unread")
	var stdin bytes.Buffer
	_, err := exchangeSSHDeployment(
		t.Context(),
		&stdin,
		bufio.NewReader(strings.NewReader("CACHED id\nPREPARE id\nREADY id\nACTIVE id\n")),
		archive,
		sshRolloutInput{Reference: "id", Size: int64(archive.Len())},
		ci.New(io.Discard),
	)
	require.NoError(t, err)
	assert.Equal(t, "must remain unread", archive.String())
	assert.Equal(t, "CONTINUE id\nACTIVATE id\n", stdin.String())
}

func TestDeploymentHandshakeUploadsMissingArchive(t *testing.T) {
	archive := "archive bytes"
	var stdin bytes.Buffer
	_, err := exchangeSSHDeployment(
		t.Context(),
		&stdin,
		bufio.NewReader(strings.NewReader("UPLOAD id\nPREPARE id\nREADY id\nACTIVE id\n")),
		strings.NewReader(archive),
		sshRolloutInput{Reference: "id", Size: int64(len(archive))},
		ci.New(io.Discard),
	)
	require.NoError(t, err)
	assert.Equal(t, archive+"CONTINUE id\nACTIVATE id\n", stdin.String())
	_, err = readRolloutAction(bufio.NewReader(strings.NewReader("UNKNOWN id\n")), "id")
	require.ErrorContains(t, err, "unexpected SSH deployment response")
}

func TestDeploymentHandshakeReusesPreparedRelease(t *testing.T) {
	for _, tc := range []struct {
		name, response, wantInput, reference string
		unchanged                            bool
	}{
		{"inactive", "REUSE id\nREADY id\nACTIVE id\n", "ACTIVATE id\n", "id", false},
		{"active", "UNCHANGED id\nprevious-id\n", "", "previous-id", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := bytes.NewBufferString("must remain unread")
			var stdin, output bytes.Buffer
			result, err := exchangeSSHDeployment(t.Context(), &stdin,
				bufio.NewReader(strings.NewReader(tc.response)), archive,
				sshRolloutInput{Reference: "id", Deployment: "happy-euclid"}, ci.New(&output))
			require.NoError(t, err)
			assert.Equal(t, tc.wantInput, stdin.String())
			assert.Equal(t, "must remain unread", archive.String())
			assert.Equal(t, tc.reference, result.Reference)
			assert.Equal(t, tc.unchanged, result.Unchanged)
			assert.True(t, result.Active)
			assert.Equal(t, "happy-euclid", result.Deployment.Reference)
			assert.NotContains(t, output.String(), "Preparing release")
			if tc.unchanged {
				assert.Empty(t, output.String())
			}
		})
	}
}

func TestSSHNamedReleaseRejectsUnsafeNames(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	e := &SSH{directory: "/var/www/shop/current"}
	for _, reference := range []string{".tar.gz", "..", ".hidden.tar.gz", "name with spaces.tar.gz", strings.Repeat("x", 129)} {
		_, err := e.rolloutArchive(t.Context(), Deployment{Reference: reference}, "missing.tar.gz", io.Discard)
		require.ErrorContains(t, err, "invalid deployment name")
	}
}
