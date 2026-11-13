package ssh

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

const sshPreparationCancellationWarning = "Cancellation does not stop the Deployment Helper on the server; it may still be running. The database may already be updated. This rollout did not activate the release. Check the server and deployment logs before retrying; retrying the same deployment can activate the prepared release."

var errSSHPreparationCanceled = errors.New("cannot finish release preparation: " + strings.TrimSuffix(sshPreparationCancellationWarning, "."))

// waitForSSHPreparation warns immediately on cancellation, even if the remote
// helper keeps the SSH streams open. Activation has not been authorized yet.
func waitForSSHPreparation(ctx context.Context, reader *bufio.Reader, reference string, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}
	warned := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(warned)
		_, _ = fmt.Fprintln(output, "Warning: "+sshPreparationCancellationWarning)
	})
	err := readRolloutMessage(reader, sshRolloutReady+" "+reference)
	stopped := stop()
	if !stopped {
		// Keep the callback from writing after the caller closes its output.
		<-warned
	}
	if ctx.Err() != nil {
		if stopped {
			_, _ = fmt.Fprintln(output, "Warning: "+sshPreparationCancellationWarning)
		}
		return errors.Join(errSSHPreparationCanceled, ctx.Err(), err)
	}
	return err
}
