package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/shopware/shopware-cli/internal/orchestration/runtime"
)

var _ runtime.ServiceStackContainers = (*DockerCompose)(nil)

const composeContainersFormat = `{"ID":{{json .ID}},"Service":{{json .Service}},"Project":{{json .Project}},"OneOff":{{json (.Label "com.docker.compose.oneoff")}}}`

type composeContainer struct {
	ID      string
	Service string
	Project string
	OneOff  string
}

// Containers identifies existing project services without changing their lifecycle.
func (p *DockerCompose) Containers(ctx context.Context) ([]runtime.ServiceContainer, error) {
	output, err := p.query(ctx, "ps", "--all", "--no-trunc", "--format", composeContainersFormat, "--orphans=false")
	if err != nil {
		return nil, err
	}
	entries, err := decodeComposeContainers(output)
	if err != nil {
		return nil, fmt.Errorf("cannot decode compose containers: %w", err)
	}
	var containers []runtime.ServiceContainer
	seen := make(map[string]bool)
	for _, entry := range entries {
		if entry.Project != p.options.ProjectName {
			continue
		}
		if strings.EqualFold(entry.OneOff, "true") {
			continue
		}
		if entry.ID == "" || entry.Service == "" {
			return nil, errors.New("invalid compose container identity")
		}
		if !seen[entry.ID] {
			containers = append(containers, runtime.ServiceContainer{Service: entry.Service, ID: entry.ID})
			seen[entry.ID] = true
		}
	}
	slices.SortFunc(containers, func(a, b runtime.ServiceContainer) int {
		if comparison := strings.Compare(a.Service, b.Service); comparison != 0 {
			return comparison
		}
		return strings.Compare(a.ID, b.ID)
	})
	return containers, nil
}

// decodeComposeContainers reads one formatted JSON object per container.
func decodeComposeContainers(data []byte) ([]composeContainer, error) {
	var entries []composeContainer
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var entry composeContainer
		if err := decoder.Decode(&entry); err != nil {
			if errors.Is(err, io.EOF) {
				return entries, nil
			}
			return nil, err
		}
		entries = append(entries, entry)
	}
}
