package project

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateJWTCommandDeprecated(t *testing.T) {
	t.Parallel()

	assert.Contains(t, projectNewJWTCmd.Deprecated, "October 2026")
}

func TestGenerateJWTPrintsDeprecationNotice(t *testing.T) {
	out := new(bytes.Buffer)
	projectNewJWTCmd.SetOut(out)
	projectNewJWTCmd.SetErr(out)
	projectNewJWTCmd.SetArgs([]string{"--help"})
	t.Cleanup(func() {
		projectNewJWTCmd.SetArgs(nil)
		projectNewJWTCmd.SetOut(os.Stderr)
		projectNewJWTCmd.SetErr(nil)
	})

	require.NoError(t, projectNewJWTCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "deprecated")
	assert.Contains(t, output, "October 2026")
}

func TestGenerateJWTEnvStillWorks(t *testing.T) {
	require.NoError(t, projectNewJWTCmd.PersistentFlags().Set("env", "true"))
	t.Cleanup(func() {
		_ = projectNewJWTCmd.PersistentFlags().Set("env", "false")
	})

	r, w, err := os.Pipe()
	require.NoError(t, err)

	stdout := os.Stdout
	os.Stdout = w
	runErr := projectNewJWTCmd.RunE(projectNewJWTCmd, []string{})
	require.NoError(t, w.Close())
	os.Stdout = stdout
	require.NoError(t, runErr)

	output, err := io.ReadAll(r)
	require.NoError(t, err)

	text := string(output)
	assert.Contains(t, text, "JWT_PRIVATE_KEY=")
	assert.Contains(t, text, "JWT_PUBLIC_KEY=")
	assert.False(t, strings.Contains(text, "deprecated"), "deprecation notice must not pollute --env stdout")
}
