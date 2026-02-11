package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"gopkg.in/yaml.v3"
)

var (
	state                        *configState
	environmentConfigErrorFormat = "could not set config value %s to %q config was loaded from the environment variables"
)

type configState struct {
	mu            sync.RWMutex
	cfgPath       string
	inner         *configData
	loadedFromEnv bool
	isReady       bool
	modified      bool
}

type configData struct {
	Account struct {
		Company int `yaml:"company"`
	} `yaml:"account"`
}

type Config struct{}

func init() {
	state = &configState{
		mu:      sync.RWMutex{},
		cfgPath: "",
		inner:   defaultConfig(),
	}
}

func defaultConfig() *configData {
	config := &configData{}
	config.Account.Company = 0
	return config
}

func InitConfig(configPath string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.isReady {
		return nil
	}

	companyId := os.Getenv("SHOPWARE_CLI_ACCOUNT_COMPANY")

	if len(companyId) > 0 {
		state.loadedFromEnv = true
		companyIdInt, err := strconv.Atoi(companyId)
		if err != nil {
			return err
		}
		state.inner.Account.Company = companyIdInt
		state.isReady = true

		return nil
	}

	if len(configPath) > 0 {
		state.cfgPath = configPath
	} else {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}

		state.cfgPath = fmt.Sprintf("%s/.shopware-cli.yml", configDir)
	}

	if _, err := os.Stat(state.cfgPath); os.IsNotExist(err) {
		if err := createNewConfig(state.cfgPath); err != nil {
			return err
		}
	}

	content, err := os.ReadFile(state.cfgPath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(content, &state.inner)
	if err != nil {
		return err
	}

	state.isReady = true
	return nil
}

func SaveConfig() error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.modified || state.loadedFromEnv {
		return nil
	}

	configFile, err := os.OpenFile(state.cfgPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	configWriter := yaml.NewEncoder(configFile)
	defer func() {
		state.modified = false
		_ = configWriter.Close()
	}()

	return configWriter.Encode(state.inner)
}

func createNewConfig(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	encoder := yaml.NewEncoder(f)
	err = encoder.Encode(defaultConfig())
	if err != nil {
		return err
	}

	return f.Close()
}

func (Config) GetAccountCompanyId() int {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.inner.Account.Company
}

func (Config) SetAccountCompanyId(id int) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.loadedFromEnv {
		return fmt.Errorf(environmentConfigErrorFormat, "account.company", strconv.Itoa(id))
	}
	state.modified = true
	state.inner.Account.Company = id
	return nil
}

func (Config) Save() error {
	return SaveConfig()
}
