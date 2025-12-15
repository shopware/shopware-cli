package curl

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

// Command represents an invocation of the curl executable.
// For example: >Curl -X POST https://127.0.0.1/test
//
//	Becomes: CurlCommand{
//	   options: [ curlOption{flag: "-X", value: "POST"} ]
//	   args: ["https://127.0.0.1/test"]
//	}.
type Command struct {
	options []curlOption
	args    []string
}

type curlOption struct {
	flag, value string
}

// Config allows to configure the CurlCommand.
type Config func(*Command)

// Method sets the http method for the curl invocation.
func Method(method string) Config {
	return func(command *Command) {
		command.addOption(curlOption{
			flag:  "-X",
			value: strings.ToUpper(method),
		})
	}
}

// BearerToken sets the "authorization:" header.
func BearerToken(token string) Config {
	return Header("Authorization", token)
}

// Args allows custom strings as arguments to the curl call.
func Args(args []string) Config {
	return func(command *Command) {
		if len(args) == 0 {
			return
		}
		if len(command.args) > 0 {
			command.args = append(command.args, args...)
		} else {
			command.args = args
		}
	}
}

// Url defines the url curl calls.
func Url(url *url.URL) Config {
	return func(command *Command) {
		if len(command.args) > 0 {
			command.args = append([]string{url.String()}, command.args...)
		} else {
			command.args = []string{url.String()}
		}
	}
}

// Header sets adds a header to the curl call.
func Header(name, value string) Config {
	return func(command *Command) {
		command.addOption(curlOption{
			flag:  "--header",
			value: fmt.Sprintf("%s: %s", name, value),
		},
		)
	}
}

// InitCurlCommand creates a new CurlCommand with the specified configs applied.
func InitCurlCommand(options ...Config) *Command {
	cmd := &Command{}
	for _, opt := range options {
		opt(cmd)
	}

	return cmd
}

func (c *Command) addOption(o curlOption) {
	c.options = append(c.options, o)
}

func (c *Command) getCmdOptions() []string {
	var cmdOptions []string
	for _, opt := range c.options {
		cmdOptions = append(cmdOptions, opt.flag, opt.value)
	}
	return append(cmdOptions, c.args...)
}

// Run runs the CurlCommand with stdin, stdout, and stderr piped through to the parent process.
func (c *Command) Run(ctx context.Context) error {
	// The user wants to execute code with custom parameters
	/* #nosec G204 */
	cmd := exec.CommandContext(ctx, "curl", c.getCmdOptions()...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
