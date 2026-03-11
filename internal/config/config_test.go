package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestParseEnvConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
	}{
		email:    "test@test.com",
		password: "test123",
	}

	t.Setenv("SHOPWARE_CLI_ACCOUNT_EMAIL", testData.email)
	t.Setenv("SHOPWARE_CLI_ACCOUNT_PASSWORD", testData.password)

	assert.NoError(t, InitConfig(""))
	assert.True(t, state.loadedFromEnv)

	confService := Config{}
	assert.Equal(t, testData.email, confService.GetAccountEmail())
	assert.Equal(t, testData.password, confService.GetAccountPassword())
}

func TestParseFileConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
	}{
		email:    "test@test.com",
		password: "test123",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)
	testConfig := filepath.Join(cwd, "testdata/.shopware-cli.yml")

	assert.NoError(t, InitConfig(testConfig))
	assert.False(t, state.loadedFromEnv)

	confService := Config{}
	assert.Equal(t, testData.email, confService.GetAccountEmail())
	assert.Equal(t, testData.password, confService.GetAccountPassword())
	assert.Equal(t, testConfig, state.cfgPath)
}

func TestSaveConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
	}{
		email:    "test@new.com",
		password: "test",
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)
	testConfig := filepath.Join(cwd, "testdata/write-test.yml")
	configBackup, err := os.ReadFile(testConfig)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.WriteFile(testConfig, configBackup, os.ModePerm))
	}()

	assert.NoError(t, InitConfig(testConfig))

	configService := Config{}

	assert.NoError(t, configService.SetAccountEmail(testData.email))

	assert.NoError(t, configService.SetAccountPassword(testData.password))

	assert.True(t, state.modified)

	assert.NoError(t, SaveConfig())

	assert.False(t, state.modified)

	newConfData, err := os.ReadFile(testConfig)
	assert.NoError(t, err)

	var newConf configData
	assert.NoError(t, yaml.Unmarshal(newConfData, &newConf))

	assert.Equal(t, testData.email, newConf.Account.Email)
	assert.Equal(t, testData.password, newConf.Account.Password)
}

func TestDontWriteEnvConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
	}{
		email:    "test@test.com",
		password: "test123",
	}

	t.Setenv("SHOPWARE_CLI_ACCOUNT_EMAIL", testData.email)
	t.Setenv("SHOPWARE_CLI_ACCOUNT_PASSWORD", testData.password)

	assert.NoError(t, InitConfig(""))
	assert.True(t, state.loadedFromEnv)

	confService := Config{}
	assert.Error(t, confService.SetAccountEmail("test@foo.com"))
	assert.Error(t, confService.SetAccountPassword("S3CR3TF4RT3St"))
}

func resetState() {
	state = &configState{
		mu:      sync.RWMutex{},
		cfgPath: "",
		inner:   defaultConfig(),
	}
}
