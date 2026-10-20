package deployment

import (
	_ "embed"

	"github.com/shopware/shopware-cli/internal/shop"
)

// Pin the official release and verify its SHA256 before execution.
const (
	cachetoolVersion = "10.0.0"
	cachetoolURL     = "https://github.com/gordalina/cachetool/releases/download/10.0.0/cachetool.phar"
	cachetoolSHA256  = "cbe90e7acdde7beafe26b592a753c2b923a99d2033e073dc55e42fba2883bd1d"
)

//go:embed ssh_cachetool.php
var sshCachetoolScript string

type sshCachetoolInput struct {
	shop.EnvironmentSSHCachetoolConfig
	Version        string `json:"version"`
	URL            string `json:"url"`
	SHA256         string `json:"sha256"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (s *SSH) cachetoolInput() (*sshCachetoolInput, error) {
	var config *shop.EnvironmentSSHCachetoolConfig
	if s.env != nil && s.env.SSH != nil {
		config = s.env.SSH.Cachetool
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.IsEnabled() {
		return nil, nil //nolint:nilnil // Nil explicitly disables this optional remote step.
	}
	return &sshCachetoolInput{
		EnvironmentSSHCachetoolConfig: *config,
		Version:                       cachetoolVersion,
		URL:                           cachetoolURL,
		SHA256:                        cachetoolSHA256,
		TimeoutSeconds:                30,
	}, nil
}
