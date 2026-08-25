package extension

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionFormatRequiresExactlyOnePathArg(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"no args", []string{"format"}, "accepts 1 arg(s), received 0"},
		{"two args", []string{"format", t.TempDir(), t.TempDir()}, "accepts 1 arg(s), received 2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := new(bytes.Buffer)
			extensionRootCmd.SetOut(out)
			extensionRootCmd.SetErr(out)
			extensionRootCmd.SetArgs(tc.args)
			t.Cleanup(func() {
				extensionRootCmd.SetArgs(nil)
				extensionRootCmd.SetOut(nil)
				extensionRootCmd.SetErr(nil)
			})

			err := extensionRootCmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}
