package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapAliasArgs_NoArgs(t *testing.T) {
	assert.Equal(t, []string{}, mapAliasArgs([]string{"shopware-cli"}))
}

func TestMapAliasArgs_RegularBinary(t *testing.T) {
	args := mapAliasArgs([]string{"shopware-cli", "project", "console", "debug:router"})

	assert.Equal(t, []string{"project", "console", "debug:router"}, args)
}

func TestMapAliasArgs_SwxAlias(t *testing.T) {
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "debug:router", "--env=prod"})

	assert.Equal(t, []string{"project", "console", "debug:router", "--env=prod"}, args)
}

func TestMapAliasArgs_SwxAliasWithoutArgs(t *testing.T) {
	args := mapAliasArgs([]string{"/usr/local/bin/swx"})

	assert.Equal(t, []string{"project", "console"}, args)
}

func TestMapAliasArgs_SwxExeAlias(t *testing.T) {
	args := mapAliasArgs([]string{"C:\\tools\\swx.exe", "cache:clear"})

	assert.Equal(t, []string{"project", "console", "cache:clear"}, args)
}

func TestMapAliasArgs_SwxCompletion(t *testing.T) {
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "completion", "bash"})

	assert.Equal(t, []string{"completion", "bash"}, args)
}

func TestMapAliasArgs_SwxInternalCompletion(t *testing.T) {
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "__complete", "cache:clear"})

	assert.Equal(t, []string{"__complete", "project", "console", "cache:clear"}, args)
}

func TestMapAliasArgs_SwxInternalCompletionNoDesc(t *testing.T) {
	args := mapAliasArgs([]string{"/usr/local/bin/swx", "__completeNoDesc", "cache:clear"})

	assert.Equal(t, []string{"__completeNoDesc", "project", "console", "cache:clear"}, args)
}

func TestCommandNameFromArgs(t *testing.T) {
	assert.Equal(t, "shopware-cli", commandNameFromArgs([]string{"/usr/local/bin/shopware-cli"}))
	assert.Equal(t, "swx", commandNameFromArgs([]string{"C:\\tools\\swx.exe"}))
	assert.Equal(t, "shopware-cli", commandNameFromArgs(nil))
}
