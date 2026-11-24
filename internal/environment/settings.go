package environment

import (
	"cmp"
	"errors"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

// Setting names one editable value of an environment.
type Setting string

const (
	SettingName         Setting = "name"
	SettingURL          Setting = "url"
	SettingHost         Setting = "host"
	SettingDirectory    Setting = "directory"
	SettingUser         Setting = "user"
	SettingPort         Setting = "port"
	SettingIdentityFile Setting = "identity_file"
	SettingPHPBinary    Setting = "php_binary"
)

// Settings are the values that describe a single-host SSH environment.
type Settings struct {
	Name string
	URL  string

	Host         string
	Directory    string
	User         string
	Port         int
	IdentityFile string
	PHPBinary    string
}

// HasOptional reports whether a setting with a default was given.
func (s Settings) HasOptional() bool {
	return s.User != "" || s.Port != 0 || s.IdentityFile != "" || s.PHPBinary != ""
}

// Config builds the environment written to the project config.
func (s Settings) Config() *shop.EnvironmentConfig {
	return &shop.EnvironmentConfig{
		Type: executor.TypeSSH,
		URL:  s.URL,
		SSH: &shop.EnvironmentSSHConfig{
			Host:         s.Host,
			User:         s.User,
			Port:         s.Port,
			Directory:    s.Directory,
			IdentityFile: s.IdentityFile,
			PHPBinary:    s.PHPBinary,
		},
	}
}

// SettingsOf reads the editable values of a configured environment. Host
// groups keep their per-host values in the config file, so only name and URL
// are editable for them.
func SettingsOf(name string, env *shop.EnvironmentConfig) (Settings, []Setting) {
	settings := Settings{Name: name, URL: env.URL}
	editable := []Setting{SettingName, SettingURL}

	if env.Type == executor.TypeSSH && env.SSH != nil && len(env.SSH.Hosts) == 0 {
		settings.Host, settings.Directory = env.SSH.Host, env.SSH.Directory
		settings.User, settings.Port = env.SSH.User, env.SSH.Port
		settings.IdentityFile, settings.PHPBinary = env.SSH.IdentityFile, env.SSH.PHPBinary
		editable = append(editable, SettingHost, SettingDirectory, SettingUser, SettingPort, SettingIdentityFile, SettingPHPBinary)
	}

	return settings, editable
}

// HostOfURL returns the host name of a shop URL, the usual SSH host.
func HostOfURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// ValidateHost accepts a bare host name or IP address.
func ValidateHost(value string) error {
	if value == "" {
		return errors.New("this cannot be empty")
	}
	if strings.ContainsAny(value, " \t\r\n/@") || strings.HasPrefix(value, "-") {
		return errors.New("enter a host name or IP address, without user or path")
	}
	return nil
}

// ValidateDirectory accepts an absolute path on the remote host.
func ValidateDirectory(value string) error {
	if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("enter an absolute path, e.g. /var/www/shop/current")
	}
	return nil
}

// Overviews lists the environments of a project config with their remembered
// status, most recently tested first.
func Overviews(cfg *shop.Config, store *Store, now time.Time) []Overview {
	overviews := make([]Overview, 0, len(cfg.Environments))
	for _, name := range slices.Sorted(maps.Keys(cfg.Environments)) {
		if env := cfg.Environments[name]; env != nil {
			overviews = append(overviews, store.Describe(name, env, now))
		}
	}
	slices.SortStableFunc(overviews, func(a, b Overview) int {
		return cmp.Or(b.TestedAt.Compare(a.TestedAt), strings.Compare(a.Name, b.Name))
	})
	return overviews
}
