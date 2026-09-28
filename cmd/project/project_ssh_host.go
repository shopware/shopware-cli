package project

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

func selectProjectSSHHost(cmd *cobra.Command, env *shop.EnvironmentConfig, singleHost bool) (*shop.EnvironmentConfig, error) {
	name, _ := cmd.Flags().GetString("ssh-host")
	if env.Type != executor.TypeSSH {
		if name != "" {
			return nil, errors.New("--ssh-host requires an SSH environment")
		}
		return env, nil
	}
	if env.SSH == nil || (len(env.SSH.Hosts) == 0 && name == "") {
		return env, nil
	}
	hosts, _, err := env.SSH.ResolveHosts()
	if err != nil {
		return nil, err
	}
	if name == "" && !singleHost {
		return env, nil
	}
	if name == "" && len(hosts) > 1 {
		return nil, errors.New("this command requires one SSH host; select it with --ssh-host")
	}
	for _, host := range hosts {
		if name == "" || name == host.Name || (host.Name == "" && name == host.Config.Host) {
			selected := *env
			selected.SSH = host.Config
			return &selected, nil
		}
	}
	return nil, fmt.Errorf("unknown SSH host %q in selected environment", name)
}

func sshHostCompletions(cmd *cobra.Command, _ []string, prefix string) ([]string, cobra.ShellCompDirective) {
	cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	env, err := cfg.ResolveEnvironment(environmentName)
	if err != nil || env.SSH == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	hosts, _, err := env.SSH.ResolveHosts()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, host := range hosts {
		if host.Name != "" && strings.HasPrefix(host.Name, prefix) {
			names = append(names, host.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
