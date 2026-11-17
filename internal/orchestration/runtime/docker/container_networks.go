package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

func configuredIPAM(raw json.RawMessage) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return false
	}
	var config struct {
		IPv4Address  string   `json:"IPv4Address"`
		IPv6Address  string   `json:"IPv6Address"`
		LinkLocalIPs []string `json:"LinkLocalIPs"`
	}
	if json.Unmarshal(raw, &config) != nil {
		return true
	}
	return config.IPv4Address != "" || config.IPv6Address != "" || len(config.LinkLocalIPs) > 0
}

// InspectNetwork returns the engine's current network state.
func (d *Runtime) InspectNetwork(ctx context.Context, name string) (runtime.ContainerNetworkState, error) {
	var list []struct {
		ID         string `json:"Id"`
		Name       string `json:"Name"`
		Containers map[string]struct {
			Name string `json:"Name"`
		} `json:"Containers"`
	}
	if err := d.inspect(ctx, "network", name, &list); err != nil {
		return runtime.ContainerNetworkState{}, err
	}
	if len(list) != 1 || list[0].ID == "" {
		return runtime.ContainerNetworkState{}, errors.New("invalid network inspection result")
	}
	s := runtime.ContainerNetworkState{ID: list[0].ID, Name: list[0].Name}
	for _, c := range list[0].Containers {
		s.Containers = append(s.Containers, c.Name)
	}
	slices.Sort(s.Containers)
	return s, nil
}

// EnsureNetwork creates a network if it does not already exist.
func (d *Runtime) EnsureNetwork(ctx context.Context, name string) error {
	_, err := d.InspectNetwork(ctx, name)
	if err == nil {
		return nil
	}
	if !errors.Is(err, runtime.ErrNotFound) {
		return err
	}
	_, _, err = d.command(ctx, "network", "create", "--", name)
	if err != nil {
		return fmt.Errorf("cannot create network: %w", err)
	}
	return nil
}

// RemoveNetwork removes an unused network if it exists.
func (d *Runtime) RemoveNetwork(ctx context.Context, name string) error {
	return d.action(ctx, "network", "rm", name, true)
}

func (d *Runtime) connect(ctx context.Context, network, container string, aliases []string) error {
	args := []string{"network", "connect"}
	for _, a := range aliases {
		args = append(args, "--alias", a)
	}
	args = append(args, "--", network, container)
	_, _, err := d.command(ctx, args...)
	return err
}

// SetNetworkAliases changes aliases and attempts to restore the prior attachment on failure.
func (d *Runtime) SetNetworkAliases(ctx context.Context, network, container string, want []string) error {
	if !validName(network) || !validName(container) {
		return errors.New("invalid resource name")
	}
	for _, a := range want {
		if !validName(a) {
			return errors.New("invalid network alias")
		}
	}
	c, err := d.container(ctx, container)
	if err != nil {
		return err
	}
	want = aliases(c, want)
	e, attached := c.NetworkSettings.Networks[network]
	if !attached {
		return d.connect(ctx, network, container, want)
	}
	old := aliases(c, e.Aliases)
	if slices.Equal(old, want) {
		return nil
	}
	if configuredIPAM(e.IPAMConfig) || len(e.Links) > 0 || len(e.DriverOpts) > 0 {
		return fmt.Errorf("%w: network endpoint settings", runtime.ErrUnsupported)
	}
	if _, _, err = d.command(ctx, "network", "disconnect", "--", network, container); err != nil {
		original := fmt.Errorf("cannot disconnect network: %w", err)
		recoveryCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		current, inspectErr := d.container(recoveryCtx, container)
		if inspectErr != nil {
			return errors.Join(original, fmt.Errorf("cannot inspect network attachment during recovery: %w", inspectErr))
		}
		if _, attached := current.NetworkSettings.Networks[network]; attached {
			return original
		}
		if recoveryErr := d.connect(recoveryCtx, network, container, old); recoveryErr != nil {
			return errors.Join(original, fmt.Errorf("cannot restore network attachment: %w", recoveryErr))
		}
		return original
	}
	if err = d.connect(ctx, network, container, want); err == nil {
		return nil
	}
	recoveryCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	recovery := d.connect(recoveryCtx, network, container, old)
	if recovery != nil {
		return errors.Join(fmt.Errorf("cannot reconnect network: %w", err), fmt.Errorf("cannot restore network attachment: %w", recovery))
	}
	return fmt.Errorf("cannot reconnect network: %w", err)
}
