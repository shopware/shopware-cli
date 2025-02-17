package config

import (
	"os"
	"path"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestParseEnvConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
		companyId       int
	}{
		companyId: 456,
	}

	t.Setenv("SHOPWARE_CLI_ACCOUNT_COMPANY", strconv.Itoa(testData.companyId))

	assert.NoError(t, InitConfig(""))
	assert.True(t, state.loadedFromEnv)

	confService := Config{}
	assert.Equal(t, testData.companyId, confService.GetAccountCompanyId())
}

func TestParseFileConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
		companyId       int
	}{
		companyId: 456,
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)
	testConfig := path.Join(cwd, "testdata/.shopware-cli.yml")

	assert.NoError(t, InitConfig(testConfig))
	assert.False(t, state.loadedFromEnv)

	confService := Config{}
	assert.Equal(t, testData.companyId, confService.GetAccountCompanyId())
	assert.Equal(t, testConfig, state.cfgPath)
}

func TestSaveConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
		companyId       int
	}{
		companyId: 111,
	}

	cwd, err := os.Getwd()
	assert.NoError(t, err)
	testConfig := path.Join(cwd, "testdata/write-test.yml")
	configBackup, err := os.ReadFile(testConfig)
	assert.NoError(t, err)
	defer func() {
		assert.NoError(t, os.WriteFile(testConfig, configBackup, os.ModePerm))
	}()

	assert.NoError(t, InitConfig(testConfig))

	configService := Config{}

	assert.NoError(t, configService.SetAccountCompanyId(testData.companyId))

	assert.True(t, state.modified)

	assert.NoError(t, SaveConfig())

	assert.False(t, state.modified)

	newConfData, err := os.ReadFile(testConfig)
	assert.NoError(t, err)

	var newConf configData
	assert.NoError(t, yaml.Unmarshal(newConfData, &newConf))

	assert.Equal(t, testData.companyId, newConf.Account.Company)
}

func TestDontWriteEnvConfig(t *testing.T) {
	defer resetState()

	testData := struct {
		email, password string
		companyId       int
	}{
		email:     "test@test.com",
		password:  "test123",
		companyId: 456,
	}

	t.Setenv("SHOPWARE_CLI_ACCOUNT_COMPANY", strconv.Itoa(testData.companyId))

	assert.NoError(t, InitConfig(""))
	assert.True(t, state.loadedFromEnv)

	confService := Config{}
	assert.Error(t, confService.SetAccountCompanyId(111))
}

func resetState() {
	state = &configState{
		mu:      sync.RWMutex{},
		cfgPath: "",
		inner:   defaultConfig(),
	}
}
