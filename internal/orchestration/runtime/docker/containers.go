package docker

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

var _ runtime.Runtime = (*Runtime)(nil)
var _ runtime.Containers = (*Runtime)(nil)
var _ runtime.ContainerNetworks = (*Runtime)(nil)

func validateCreate(o runtime.CreateContainerOptions) error {
	if !validName(o.Name) || !validName(o.Image) {
		return errors.New("invalid container name or image")
	}
	if hasControl(o.Entrypoint) || hasControl(o.User) || hasControl(o.Dir) {
		return errors.New("invalid container option")
	}
	if o.Dir != "" && !filepath.IsAbs(o.Dir) {
		return errors.New("working directory must be absolute")
	}
	switch o.RestartPolicy {
	case "", "no", "always", "unless-stopped", "on-failure":
	default:
		return fmt.Errorf("%w: restart policy", runtime.ErrUnsupported)
	}
	for _, m := range o.Mounts {
		if err := validateMount(m); err != nil {
			return err
		}
	}
	for k := range o.Env {
		if !validEnvKey(k) {
			return errors.New("invalid environment key")
		}
	}
	for k := range o.Labels {
		if !validName(k) || strings.Contains(k, "=") {
			return errors.New("invalid label key")
		}
	}
	seen := map[string]bool{}
	for _, n := range o.Networks {
		if !validName(n.Name) || seen[n.Name] {
			return errors.New("invalid or duplicate network")
		}
		seen[n.Name] = true
		for _, a := range n.Aliases {
			if !validName(a) {
				return errors.New("invalid network alias")
			}
		}
	}
	for _, p := range o.Ports {
		if p.ContainerPort == 0 || (p.Protocol != "" && p.Protocol != "tcp" && p.Protocol != "udp") || (p.HostIP != "" && net.ParseIP(p.HostIP) == nil) {
			return errors.New("invalid port binding")
		}
	}
	return nil
}

// CreateContainer creates a persistent container and attaches any additional networks.
func (d *Runtime) CreateContainer(ctx context.Context, o runtime.CreateContainerOptions) (string, error) {
	if err := validateCreate(o); err != nil {
		return "", err
	}
	args := []string{"container", "create", "--name", o.Name}
	if o.Entrypoint != "" {
		args = append(args, "--entrypoint", o.Entrypoint)
	}
	if o.User != "" {
		args = append(args, "--user", o.User)
	}
	if o.Dir != "" {
		args = append(args, "--workdir", o.Dir)
	}
	if o.RestartPolicy != "" {
		args = append(args, "--restart", o.RestartPolicy)
	}
	for _, m := range o.Mounts {
		args = append(args, "--mount", mountValue(m))
	}
	for _, k := range slices.Sorted(maps.Keys(o.Env)) {
		args = append(args, "--env", k+"="+o.Env[k])
	}
	for _, k := range slices.Sorted(maps.Keys(o.Labels)) {
		args = append(args, "--label", k+"="+o.Labels[k])
	}
	for _, p := range o.Ports {
		protocol := p.Protocol
		if protocol == "" {
			protocol = "tcp"
		}
		host := strconv.Itoa(int(p.HostPort))
		if p.HostPort == 0 {
			host = ""
		}
		if p.HostIP != "" {
			ip := p.HostIP
			if strings.Contains(ip, ":") {
				ip = "[" + ip + "]"
			}
			host = ip + ":" + host
		}
		args = append(args, "--publish", host+":"+strconv.Itoa(int(p.ContainerPort))+"/"+protocol)
	}
	if len(o.Networks) > 0 {
		args = append(args, "--network", o.Networks[0].Name)
		for _, a := range o.Networks[0].Aliases {
			args = append(args, "--network-alias", a)
		}
	}
	args = append(args, "--", o.Image)
	args = append(args, o.Args...)
	out, _, err := d.command(ctx, args...)
	id := strings.TrimSpace(out)
	if len(strings.Fields(id)) != 1 || hasControl(id) {
		id = ""
	}
	if err != nil {
		return id, fmt.Errorf("cannot create container: %w", err)
	}
	if id == "" {
		return "", errors.New("docker returned an empty container ID")
	}
	for _, n := range o.Networks[min(1, len(o.Networks)):] {
		if err := d.connect(ctx, n.Name, id, n.Aliases); err != nil {
			return id, fmt.Errorf("cannot attach container to network: %w", err)
		}
	}
	return id, nil
}

func (d *Runtime) action(ctx context.Context, kind, verb, name string, ignoreAbsent bool, extra ...string) error {
	if !validName(name) {
		return errors.New("invalid resource name")
	}
	args := append([]string{kind, verb}, extra...)
	args = append(args, "--", name)
	_, diag, err := d.command(ctx, args...)
	if err != nil {
		if absent(diag, kind, verb, name) && ctx.Err() == nil {
			if ignoreAbsent {
				return nil
			}
			return errors.Join(runtime.ErrNotFound, fmt.Errorf("cannot %s %s: %w", verb, kind, err))
		}
		return fmt.Errorf("cannot %s %s: %w", verb, kind, err)
	}
	return nil
}

// StartContainer starts an existing container.
func (d *Runtime) StartContainer(ctx context.Context, name string) error {
	return d.action(ctx, "container", "start", name, false)
}

// StopContainer stops an existing container.
func (d *Runtime) StopContainer(ctx context.Context, name string) error {
	return d.action(ctx, "container", "stop", name, false)
}

// RestartContainer restarts an existing container.
func (d *Runtime) RestartContainer(ctx context.Context, name string) error {
	return d.action(ctx, "container", "restart", name, false)
}

// RemoveContainer forcefully removes a container and retains named volumes.
func (d *Runtime) RemoveContainer(ctx context.Context, name string) error {
	return d.action(ctx, "container", "rm", name, true, "--force")
}
