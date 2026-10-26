package verifier

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgressReportFormats(t *testing.T) {
	for _, tc := range []struct {
		format string
		output io.Writer
	}{
		{"", os.Stderr},
		{"summary", os.Stderr},
		{"SUMMARY", os.Stderr},
		{"json", io.Discard},
		{"github", io.Discard},
		{"gitlab", io.Discard},
		{"junit", io.Discard},
		{"markdown", io.Discard},
	} {
		t.Run(tc.format, func(t *testing.T) {
			assert.Equal(t, tc.output, NewProgress(tc.format).output)
		})
	}
}

func TestProgressReportsOperationLifecycle(t *testing.T) {
	for _, tc := range []struct {
		state string
		err   error
	}{
		{"Finished", nil},
		{"Failed", errors.New("operation failed")},
	} {
		t.Run(tc.state, func(t *testing.T) {
			var output bytes.Buffer
			progress := &Progress{output: &output}
			finish := progress.Start("copying files")
			assert.Equal(t, "Running copying files...\n", output.String())

			finish(tc.err)
			assert.Regexp(t, `^Running copying files\.\.\.\n`+
				tc.state+` copying files \([0-9]+\.[0-9]s\)\n$`, output.String())
		})
	}
}
