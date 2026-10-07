package skills

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddArgs(t *testing.T) {
	src := "https://github.com/shopware/shopware-cli/tree/0.18.3/skills/shopware-cli"
	got := strings.Join(AddArgs(src, "claude-code", true, true), " ")
	want := "npx --yes skills@" + skillsVersion + " add " + src + " --agent claude-code --global -y"
	assert.Equal(t, want, got)

	got = strings.Join(AddArgs("a", "claude-code", false, false), " ")
	assert.NotContains(t, got, "--global")
	assert.NotContains(t, got, " -y")
}

func TestOwnerRepo(t *testing.T) {
	assert.Equal(t, "shopware/deployment-helper", OwnerRepo("https://github.com/shopware/deployment-helper"))
	assert.Equal(t, "shopware/deployment-helper", OwnerRepo("https://github.com/shopware/deployment-helper.git"))
}

func TestSourceURL(t *testing.T) {
	assert.Equal(t,
		"https://github.com/shopware/shopware-cli/tree/0.18.3/skills/shopware-cli",
		SourceURL("https://github.com/shopware/shopware-cli", "0.18.3", "shopware-cli"))
	assert.Equal(t,
		"https://github.com/shopware/deployment-helper/tree/0.1.7/skills/deployment-helper",
		SourceURL("https://github.com/shopware/deployment-helper.git", "0.1.7", "deployment-helper"))
}

func TestTagNames(t *testing.T) {
	got := tagNames(strings.Split(strings.TrimSpace(`
deadbeef	refs/tags/0.1.7
deadbeef	refs/tags/0.1.7^{}
deadbeef	refs/tags/v0.1.9
`), "\n"))
	assert.Equal(t, []string{"0.1.7", "0.1.7", "v0.1.9"}, got)
}

func TestLatestStableTag(t *testing.T) {
	got, err := latestStableTag([]string{"0.1.5", "0.1.7", "0.1.6", "0.2.0-RC1"})
	require.NoError(t, err)
	assert.Equal(t, "0.1.7", got) // highest stable; 0.2.0-RC1 is ignored

	_, err = latestStableTag([]string{"1.0.0-beta"})
	assert.Error(t, err) // only pre-releases → no stable tag
}

func TestMatchTag(t *testing.T) {
	tags := []string{"0.1.10", "v0.1.9", "0.18.5", "v0.18.5"}

	got, ok := matchTag(tags, "0.1.9") // want bare, only v-tag exists
	require.True(t, ok)
	assert.Equal(t, "v0.1.9", got)

	got, ok = matchTag(tags, "v0.1.10") // want v, only bare tag exists
	require.True(t, ok)
	assert.Equal(t, "0.1.10", got)

	_, ok = matchTag(tags, "9.9.9")
	assert.False(t, ok)
}

func TestInterpretCompatOutput(t *testing.T) {
	var out bytes.Buffer

	// Compatible → no error, no noise.
	require.NoError(t, interpretCompatOutput([]byte(`{"compatible":true}`), "", "dh", "0.1.7", nil, &out))

	// Incompatible → error, reasons rendered, no raw JSON shown.
	out.Reset()
	err := interpretCompatOutput(
		[]byte(`{"compatible":false,"errors":["composer.json not found — not a Shopware project"]}`),
		"", "dh", "0.1.7", errors.New("exit 1"), &out)
	assert.ErrorContains(t, err, "not compatible")
	assert.Contains(t, out.String(), "composer.json not found — not a Shopware project")
	assert.NotContains(t, out.String(), "{", "raw JSON must not reach the user")

	// No verdict produced → could not run (environment problem).
	out.Reset()
	err = interpretCompatOutput([]byte("bash: php: command not found"), "php: command not found", "dh", "0.1.7", errors.New("exit 127"), &out)
	assert.ErrorContains(t, err, "cannot run")
}

func TestRunCompatCheckRequiresPHP(t *testing.T) {
	t.Setenv("PATH", "") // hide php; the check must fail fast, before any network

	err := RunCompatCheck(t.Context(), "shopware/deployment-helper", "deployment-helper", "0.1.7", t.TempDir(), io.Discard)
	assert.ErrorContains(t, err, "PHP is not on PATH")
}
