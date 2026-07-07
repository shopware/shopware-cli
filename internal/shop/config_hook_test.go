package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfigDeploymentHookUnmarshalString(t *testing.T) {
	var d ConfigDeployment
	require.NoError(t, yaml.Unmarshal([]byte("hooks:\n  pre: |\n    echo hi\n"), &d))

	assert.Equal(t, []ConfigDeploymentHookStep{{Script: "echo hi\n"}}, d.Hooks.Pre.Steps)
}

func TestConfigDeploymentHookUnmarshalEmptyString(t *testing.T) {
	var d ConfigDeployment
	require.NoError(t, yaml.Unmarshal([]byte("hooks:\n  pre: ''\n"), &d))

	assert.Nil(t, d.Hooks.Pre.Steps)
}

func TestConfigDeploymentHookUnmarshalStepList(t *testing.T) {
	yml := "hooks:\n" +
		"  post:\n" +
		"    - title: Warm up the cache\n" +
		"      script: bin/console cache:warmup\n" +
		"    - title: Notify the team\n" +
		"      script: ./notify.sh\n"

	var d ConfigDeployment
	require.NoError(t, yaml.Unmarshal([]byte(yml), &d))

	assert.Equal(t, []ConfigDeploymentHookStep{
		{Title: "Warm up the cache", Script: "bin/console cache:warmup"},
		{Title: "Notify the team", Script: "./notify.sh"},
	}, d.Hooks.Post.Steps)
}

func TestConfigDeploymentHookUnmarshalStringListShorthand(t *testing.T) {
	yml := "hooks:\n" +
		"  pre-update:\n" +
		"    - echo first\n" +
		"    - echo second\n"

	var d ConfigDeployment
	require.NoError(t, yaml.Unmarshal([]byte(yml), &d))

	assert.Equal(t, []ConfigDeploymentHookStep{
		{Script: "echo first"},
		{Script: "echo second"},
	}, d.Hooks.PreUpdate.Steps)
}

func TestConfigDeploymentHookUnmarshalInvalid(t *testing.T) {
	var d ConfigDeployment
	err := yaml.Unmarshal([]byte("hooks:\n  pre:\n    foo: bar\n"), &d)

	assert.Error(t, err)
}
