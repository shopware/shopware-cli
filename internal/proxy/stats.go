package proxy

import (
	"context"
	"strconv"
	"strings"
)

// InstanceStats reports how many shops are currently running behind the shared
// proxy and the total memory their containers use. It is a rough resource
// indicator for the overview: shops are counted as the distinct docker compose
// projects among the containers attached to the shared proxy network, and
// memory is summed across every container belonging to those projects
// (including ones not on the proxy network, e.g. the database).
func InstanceStats(ctx context.Context) (shops int, memBytes int64, err error) {
	out, err := runDocker(ctx, "ps", "--filter", "network="+NetworkName, "--format", `{{.Label "com.docker.compose.project"}}`)
	if err != nil {
		return 0, 0, err
	}

	projects := map[string]bool{}
	for _, p := range strings.Split(strings.TrimSpace(out), "\n") {
		if p = strings.TrimSpace(p); p != "" {
			projects[p] = true
		}
	}
	if len(projects) == 0 {
		return 0, 0, nil
	}

	// Memory is best-effort: the count alone is still useful if stats fail, so
	// a stats error is deliberately ignored.
	if stats, statsErr := runDocker(ctx, "stats", "--no-stream", "--format", "{{.Name}}\t{{.MemUsage}}"); statsErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(stats), "\n") {
			name, usage, ok := strings.Cut(line, "\t")
			if !ok || !containerBelongsToProject(name, projects) {
				continue
			}
			if b, ok := parseDockerMemUsage(usage); ok {
				memBytes += b
			}
		}
	}

	return len(projects), memBytes, nil
}

// containerBelongsToProject reports whether a compose container name (e.g.
// "my-shop-web-1") belongs to one of the given compose projects. The trailing
// dash guards against a prefix collision (e.g. "my-shop" vs "my-shop-2").
func containerBelongsToProject(containerName string, projects map[string]bool) bool {
	for project := range projects {
		if strings.HasPrefix(containerName, project+"-") {
			return true
		}
	}
	return false
}

// parseDockerMemUsage parses the used side of a docker stats MemUsage value
// ("45.6MiB / 7.6GiB") into bytes.
func parseDockerMemUsage(usage string) (int64, bool) {
	used, _, _ := strings.Cut(usage, "/")
	used = strings.TrimSpace(used)

	var multiplier float64 = 1
	switch {
	case strings.HasSuffix(used, "TiB"):
		multiplier, used = 1<<40, strings.TrimSuffix(used, "TiB")
	case strings.HasSuffix(used, "GiB"):
		multiplier, used = 1<<30, strings.TrimSuffix(used, "GiB")
	case strings.HasSuffix(used, "MiB"):
		multiplier, used = 1<<20, strings.TrimSuffix(used, "MiB")
	case strings.HasSuffix(used, "KiB"):
		multiplier, used = 1<<10, strings.TrimSuffix(used, "KiB")
	case strings.HasSuffix(used, "B"):
		used = strings.TrimSuffix(used, "B")
	default:
		return 0, false
	}

	n, err := strconv.ParseFloat(strings.TrimSpace(used), 64)
	if err != nil {
		return 0, false
	}
	return int64(n * multiplier), true
}
