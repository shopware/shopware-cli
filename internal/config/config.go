package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/caarlos0/env/v9"
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
		Email    string `env:"SHOPWARE_CLI_ACCOUNT_EMAIL" yaml:"email"`
		Password string `env:"SHOPWARE_CLI_ACCOUNT_PASSWORD" yaml:"password"`
		Company  int    `env:"SHOPWARE_CLI_ACCOUNT_COMPANY" yaml:"company"`
	} `yaml:"account"`
}

type ExtensionConfig struct {
	Name             string
	Namespace        string
	ComposerPackage  string
	ShopwareVersion  string
	Description      string
	License          string
	Label            string
	ManufacturerLink string
	SupportLink      string
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
	config.Account.Email = ""
	config.Account.Password = ""
	config.Account.Company = 0
	return config
}

func InitConfig(configPath string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.isReady {
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

	err := env.Parse(state.inner)
	if err != nil {
		return err
	}
	if len(state.inner.Account.Email) > 0 {
		state.loadedFromEnv = true

		state.isReady = true

		return nil
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

func (Config) GetAccountEmail() string {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.inner.Account.Email
}

func (Config) GetAccountPassword() string {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.inner.Account.Password
}

func (Config) GetAccountCompanyId() int {
	state.mu.RLock()
	defer state.mu.RUnlock()
	return state.inner.Account.Company
}

func (Config) SetAccountEmail(email string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.loadedFromEnv {
		return fmt.Errorf(environmentConfigErrorFormat, "account.email", email)
	}
	state.modified = true
	state.inner.Account.Email = email
	return nil
}

func (Config) SetAccountPassword(password string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.loadedFromEnv {
		return fmt.Errorf(environmentConfigErrorFormat, "account.password", "***")
	}
	state.modified = true
	state.inner.Account.Password = password
	return nil
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
