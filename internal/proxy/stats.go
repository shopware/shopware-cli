package proxy

import (
	"context"
	"strconv"
	"strings"
)

// InstanceStats reports how many shops are currently running behind the shared
// proxy and the total memory their containers use. It is a rough resource
// indicator for the overview. A shop is a docker compose project that has at
// least one container attached to the shared proxy network; memory is summed
// across every running container of those projects (including ones not on the
// proxy network, e.g. the database), matched by compose project rather than by
// name prefix so unrelated containers are never counted.
func InstanceStats(ctx context.Context) (shops int, memBytes int64, err error) {
	// One pass over running containers: their name, compose project and the
	// networks they are on.
	out, err := runDocker(ctx, "ps", "--format", "{{.Names}}\t{{.Label \"com.docker.compose.project\"}}\t{{.Networks}}")
	if err != nil {
		return 0, 0, err
	}

	projectOf := map[string]string{} // container name -> compose project
	proxyProjects := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		name, project, networks := fields[0], fields[1], fields[2]
		if project == "" {
			continue
		}
		projectOf[name] = project
		for _, n := range strings.Split(networks, ",") {
			if strings.TrimSpace(n) == NetworkName {
				proxyProjects[project] = true
			}
		}
	}
	if len(proxyProjects) == 0 {
		return 0, 0, nil
	}

	// Memory is best-effort: the count alone is still useful if stats fail, so
	// a stats error is deliberately ignored.
	if stats, statsErr := runDocker(ctx, "stats", "--no-stream", "--format", "{{.Name}}\t{{.MemUsage}}"); statsErr == nil {
		for _, line := range strings.Split(strings.TrimSpace(stats), "\n") {
			name, usage, ok := strings.Cut(line, "\t")
			if !ok || !proxyProjects[projectOf[name]] {
				continue
			}
			if b, ok := parseDockerMemUsage(usage); ok {
				memBytes += b
			}
		}
	}

	return len(proxyProjects), memBytes, nil
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
