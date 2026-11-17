package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

type containerJSON struct {
	ID           string `json:"Id"`
	Name         string `json:"Name"`
	RestartCount int    `json:"RestartCount"`
	Config       struct {
		Labels map[string]string `json:"Labels"`
		Image  string            `json:"Image"`
	} `json:"Config"`
	State struct {
		Status  string `json:"Status"`
		Running bool   `json:"Running"`
	} `json:"State"`
	NetworkSettings struct {
		Networks map[string]endpointJSON      `json:"Networks"`
		Ports    map[string][]portBindingJSON `json:"Ports"`
	} `json:"NetworkSettings"`
}
type portBindingJSON struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}
type endpointJSON struct {
	Aliases    []string          `json:"Aliases"`
	IPAMConfig json.RawMessage   `json:"IPAMConfig"`
	Links      []string          `json:"Links"`
	DriverOpts map[string]string `json:"DriverOpts"`
	MacAddress string            `json:"MacAddress"`
}

func (d *Runtime) container(ctx context.Context, name string) (containerJSON, error) {
	var list []containerJSON
	err := d.inspect(ctx, "container", name, &list)
	if err != nil {
		return containerJSON{}, err
	}
	if len(list) != 1 || list[0].ID == "" {
		return containerJSON{}, errors.New("invalid container inspection result")
	}
	return list[0], nil
}

func aliases(c containerJSON, values []string) []string {
	var out []string
	name := strings.TrimPrefix(c.Name, "/")
	for _, a := range values {
		if a != name && a != c.ID && (len(a) < 12 || !strings.HasPrefix(c.ID, a)) {
			out = append(out, a)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// containerStatus maps Docker states to the portable lifecycle vocabulary.
func containerStatus(status string) runtime.ContainerStatus {
	switch status {
	case "created":
		return runtime.ContainerCreated
	case "running":
		return runtime.ContainerRunning
	case "exited":
		return runtime.ContainerStopped
	case "paused":
		return runtime.ContainerPaused
	case "restarting":
		return runtime.ContainerRestarting
	case "dead":
		return runtime.ContainerFailed
	default:
		return runtime.ContainerUnknown
	}
}

// InspectContainer returns the engine's current container state.
func (d *Runtime) InspectContainer(ctx context.Context, name string) (runtime.ContainerState, error) {
	c, err := d.container(ctx, name)
	if err != nil {
		return runtime.ContainerState{}, err
	}
	ports, err := containerPorts(c)
	if err != nil {
		return runtime.ContainerState{}, fmt.Errorf("cannot decode container ports: %w", err)
	}
	s := runtime.ContainerState{ID: c.ID, Name: strings.TrimPrefix(c.Name, "/"), Image: c.Config.Image, Ports: ports, Status: containerStatus(c.State.Status), Running: c.State.Running, RestartCount: c.RestartCount, Labels: c.Config.Labels}
	for n, e := range c.NetworkSettings.Networks {
		s.Networks = append(s.Networks, runtime.ContainerNetwork{Name: n, Aliases: aliases(c, e.Aliases)})
	}
	slices.SortFunc(s.Networks, func(a, b runtime.ContainerNetwork) int { return strings.Compare(a.Name, b.Name) })
	return s, nil
}

// containerPorts decodes published bindings without parsing formatted CLI output.
func containerPorts(c containerJSON) ([]runtime.ContainerPort, error) {
	var ports []runtime.ContainerPort
	for key, bindings := range c.NetworkSettings.Ports {
		number, protocol, ok := strings.Cut(key, "/")
		target, err := strconv.ParseUint(number, 10, 16)
		if !ok || err != nil || target == 0 || (protocol != "tcp" && protocol != "udp" && protocol != "sctp") {
			return nil, fmt.Errorf("invalid container port %q", key)
		}
		for _, binding := range bindings {
			host, err := strconv.ParseUint(binding.HostPort, 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid host port %q: %w", binding.HostPort, err)
			}
			ports = append(ports, runtime.ContainerPort{HostIP: binding.HostIP, HostPort: uint16(host), ContainerPort: uint16(target), Protocol: protocol})
		}
	}
	slices.SortFunc(ports, func(a, b runtime.ContainerPort) int {
		if a.ContainerPort != b.ContainerPort {
			return int(a.ContainerPort) - int(b.ContainerPort)
		}
		if a.Protocol != b.Protocol {
			return strings.Compare(a.Protocol, b.Protocol)
		}
		if a.HostIP != b.HostIP {
			return strings.Compare(a.HostIP, b.HostIP)
		}
		return int(a.HostPort) - int(b.HostPort)
	})
	return ports, nil
}
