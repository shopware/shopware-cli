package ssh

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cancellationWriter struct {
	bytes.Buffer
	cancel context.CancelFunc
}

func (w *cancellationWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(string(p), "CONTINUE") {
		w.cancel()
	}
	return n, err
}

type cancellationWarningWriter struct {
	warnings chan string
}

func (w cancellationWarningWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), "Warning:") {
		w.warnings <- string(p)
	}
	return len(p), nil
}

func TestPreparationCancellationWarnsBeforeRemoteFinishes(t *testing.T) {
	for _, response := range []string{"READY id\n", ""} {
		t.Run(response, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			remote, server := io.Pipe()
			t.Cleanup(func() { assert.NoError(t, remote.Close()) })
			t.Cleanup(func() { assert.NoError(t, server.Close()) })
			stdin := &cancellationWriter{cancel: cancel}
			warnings := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				_, err := exchangeSSHDeployment(ctx, stdin,
					bufio.NewReader(io.MultiReader(strings.NewReader("CACHED id\nPREPARE id\n"), remote)),
					nil, sshRolloutInput{Reference: "id"}, cancellationWarningWriter{warnings})
				done <- err
			}()

			// The server has not sent READY or closed its streams yet.
			select {
			case warning := <-warnings:
				assert.Contains(t, warning, "database may already be updated")
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation did not warn while the remote helper was running")
			}
			select {
			case err := <-done:
				t.Fatalf("protocol returned before the remote finished: %v", err)
			default:
			}
			if response != "" {
				_, err := io.WriteString(server, response)
				require.NoError(t, err)
			}
			require.NoError(t, server.Close())
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
				require.ErrorIs(t, err, errSSHPreparationCanceled)
				assert.Equal(t, "CONTINUE id\n", stdin.String())
			case <-time.After(5 * time.Second):
				t.Fatal("protocol did not return after the remote finished")
			}
		})
	}
}

func TestPreparationWithoutCancellation(t *testing.T) {
	for _, response := range []string{"READY id\n", ""} {
		t.Run(response, func(t *testing.T) {
			var output bytes.Buffer
			err := waitForSSHPreparation(t.Context(), bufio.NewReader(strings.NewReader(response)), "id", &output)
			if response == "" {
				require.ErrorIs(t, err, io.EOF)
			} else {
				require.NoError(t, err)
			}
			assert.Empty(t, output.String())
			assert.NotErrorIs(t, err, errSSHPreparationCanceled)
		})
	}
}
