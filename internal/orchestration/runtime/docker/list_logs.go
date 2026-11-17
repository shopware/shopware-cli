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

// ListContainers returns current summaries for the selected exact IDs.
func (d *Runtime) ListContainers(ctx context.Context, opts runtime.ListContainersOptions) ([]runtime.ContainerSummary, error) {
	for _, id := range opts.IDs {
		if !validName(id) {
			return nil, errors.New("invalid container ID")
		}
	}
	out, _, err := d.command(ctx, "container", "ls", "--all", "--no-trunc", "--format", "{{.ID}}")
	if err != nil {
		return nil, fmt.Errorf("cannot list containers: %w", err)
	}
	ids := strings.Fields(out)
	if len(opts.IDs) > 0 {
		ids = slices.DeleteFunc(ids, func(id string) bool { return !slices.Contains(opts.IDs, id) })
	}
	if len(ids) == 0 {
		return []runtime.ContainerSummary{}, nil
	}
	args := append([]string{"container", "inspect", "--"}, ids...)
	out, diagnostic, err := d.command(ctx, args...)
	expected := make(map[string]bool, len(ids))
	for _, id := range ids {
		expected[id] = true
	}
	if err != nil {
		if diagnostic == "" || ctx.Err() != nil {
			return nil, fmt.Errorf("cannot inspect containers: %w", err)
		}
		for _, line := range strings.Split(diagnostic, "\n") {
			found := false
			for id := range expected {
				if absent(line, "container", "inspect", id) {
					delete(expected, id)
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("cannot inspect containers: %w", err)
			}
		}
	}
	var entries []containerJSON
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		return nil, fmt.Errorf("cannot decode container inspections: %w", err)
	}
	result := make([]runtime.ContainerSummary, 0, len(entries))
	for _, c := range entries {
		if !expected[c.ID] {
			return nil, errors.New("invalid container inspection result")
		}
		delete(expected, c.ID)
		ports, err := containerPorts(c)
		if err != nil {
			return nil, fmt.Errorf("cannot decode container ports: %w", err)
		}
		result = append(result, runtime.ContainerSummary{ID: c.ID, Name: strings.TrimPrefix(c.Name, "/"), Image: c.Config.Image, Status: containerStatus(c.State.Status), Ports: ports})
	}
	if len(expected) != 0 {
		return nil, errors.New("incomplete container inspection result")
	}
	return result, nil
}

// Logs streams container output without changing the container lifecycle.
func (d *Runtime) Logs(ctx context.Context, id string, opts runtime.ContainerLogOptions) error {
	if !validName(id) {
		return errors.New("invalid container name")
	}
	if opts.Tail < 0 {
		return errors.New("log tail must not be negative")
	}
	args := []string{"container", "logs"}
	if opts.Follow {
		args = append(args, "--follow")
	}
	if opts.Tail > 0 {
		args = append(args, "--tail", strconv.Itoa(opts.Tail))
	}
	args = append(args, "--", id)
	if err := d.run(ctx, opts.Stdout, opts.Stderr, args...); err != nil {
		return fmt.Errorf("cannot read container logs: %w", err)
	}
	return nil
}
