package curl

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInitCurlCommand(t *testing.T) {
	t.Run("empty command", func(t *testing.T) {
		cmd := InitCurlCommand()
		assert.Empty(t, cmd.options)
		assert.Empty(t, cmd.args)
	})

	t.Run("with method", func(t *testing.T) {
		cmd := InitCurlCommand(Method("POST"))
		assert.Len(t, cmd.options, 1)
		assert.Equal(t, "-X", cmd.options[0].flag)
		assert.Equal(t, "POST", cmd.options[0].value)
	})

	t.Run("with bearer token", func(t *testing.T) {
		cmd := InitCurlCommand(BearerToken("test-token"))
		assert.Len(t, cmd.options, 1)
		assert.Equal(t, "--header", cmd.options[0].flag)
		assert.Equal(t, "Authorization: test-token", cmd.options[0].value)
	})

	t.Run("with custom args", func(t *testing.T) {
		cmd := InitCurlCommand(Args([]string{"-v"}))
		assert.Empty(t, cmd.options)
		assert.Equal(t, []string{"-v"}, cmd.args)
	})

	t.Run("with URL", func(t *testing.T) {
		u, _ := url.Parse("https://example.com")
		cmd := InitCurlCommand(Url(u))
		assert.Empty(t, cmd.options)
		assert.Equal(t, []string{"https://example.com"}, cmd.args)
	})

	t.Run("with header", func(t *testing.T) {
		cmd := InitCurlCommand(Header("Content-Type", "application/json"))
		assert.Len(t, cmd.options, 1)
		assert.Equal(t, "--header", cmd.options[0].flag)
		assert.Equal(t, "Content-Type: application/json", cmd.options[0].value)
	})

	t.Run("with multiple options", func(t *testing.T) {
		u, _ := url.Parse("https://example.com")
		cmd := InitCurlCommand(
			Method("POST"),
			Url(u),
			Header("Content-Type", "application/json"),
			BearerToken("test-token"),
		)

		assert.Len(t, cmd.options, 3)
		assert.Equal(t, []string{"https://example.com"}, cmd.args)

		cmdOptions := cmd.getCmdOptions()
		expectedOptions := []string{
			"-X", "POST",
			"--header", "Content-Type: application/json",
			"--header", "Authorization: test-token",
			"https://example.com",
		}
		assert.Equal(t, expectedOptions, cmdOptions)
	})
}

func TestCommand_getCmdOptions(t *testing.T) {
	t.Run("empty command", func(t *testing.T) {
		cmd := &Command{}
		assert.Empty(t, cmd.getCmdOptions())
	})

	t.Run("with options and args", func(t *testing.T) {
		cmd := &Command{
			options: []curlOption{
				{flag: "-X", value: "POST"},
				{flag: "--header", value: "Content-Type: application/json"},
			},
			args: []string{"https://example.com"},
		}

		expected := []string{
			"-X", "POST",
			"--header", "Content-Type: application/json",
			"https://example.com",
		}
		assert.Equal(t, expected, cmd.getCmdOptions())
	})
}

func TestArgs_WithExistingArgs(t *testing.T) {
	cmd := &Command{args: []string{"existing"}}
	Args([]string{"new"})(cmd)
	assert.Equal(t, []string{"existing", "new"}, cmd.args)
}

func TestUrl_WithExistingArgs(t *testing.T) {
	cmd := &Command{args: []string{"existing"}}
	u, _ := url.Parse("https://example.com")
	Url(u)(cmd)
	assert.Equal(t, []string{"https://example.com", "existing"}, cmd.args)
}
