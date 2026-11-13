package ssh

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHActivationProtocol(t *testing.T) {
	for _, action := range []string{"UPLOAD", "CACHED"} {
		var stdin bytes.Buffer
		_, err := exchangeSSHDeployment(t.Context(), &stdin,
			bufio.NewReader(strings.NewReader(action+" id\n")), nil,
			sshRolloutInput{Reference: "id", Action: sshDeploymentActivate}, io.Discard)
		require.ErrorContains(t, err, "unexpected archive operation")
		assert.Empty(t, stdin.String())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdin bytes.Buffer
	_, err := exchangeSSHDeployment(ctx, &stdin, bufio.NewReader(strings.NewReader("REUSE id\nREADY id\n")),
		nil, sshRolloutInput{Reference: "id", Action: sshDeploymentActivate}, io.Discard)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, stdin.String())
}
