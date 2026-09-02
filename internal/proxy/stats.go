package proxy

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/envfile"
)

// InstanceInfo describes one project registered with the shared proxy: whether
// it is currently running and, for running instances, its approximate memory
// use and uptime.
type InstanceInfo struct {
	Name     string // friendly project name (the project directory basename)
	URL      string // https://<hostname>
	Running  bool
	MemBytes int64         // 0 when not running
	Uptime   time.Duration // 0 when not running
}

// InstanceStats reports every project registered with the shared proxy. Running
// instances come first (then stopped ones, in registry order), each carrying an
// approximate memory total and uptime while running. combinedMem is the summed
// memory of the running instances. Memory and uptime are best-effort — a docker
// stats/inspect error yields a partial result rather than failing the overview.
func InstanceStats(ctx context.Context) (instances []InstanceInfo, combinedMem int64, err error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, 0, err
	}
	if len(reg.Projects) == 0 {
		return nil, 0, nil
	}

	// Running containers: their name, compose project, and id (for inspect).
	psOut, err := runDocker(ctx, "ps", "--format", "{{.ID}}\t{{.Names}}\t{{.Label \"com.docker.compose.project\"}}")
	if err != nil {
		return nil, 0, err
	}

	projectOfContainer := map[string]string{} // container name -> compose project
	runningProjects := map[string]bool{}
	var runningIDs []string
	for _, line := range strings.Split(strings.TrimSpace(psOut), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || fields[2] == "" {
			continue
		}
		id, name, project := fields[0], fields[1], fields[2]
		runningIDs = append(runningIDs, id)
		projectOfContainer[name] = project
		runningProjects[project] = true
	}

	memByProject := memoryByProject(ctx, projectOfContainer)
	startByProject := earliestStartByProject(ctx, runningIDs)

	now := time.Now()
	for _, entry := range reg.Projects {
		project := ComposeProjectName(entry.ProjectRoot)
		info := InstanceInfo{
			Name: filepath.Base(entry.ProjectRoot),
			URL:  "https://" + entry.Hostname,
		}
		if runningProjects[project] {
			info.Running = true
			info.MemBytes = memByProject[project]
			if start, ok := startByProject[project]; ok {
				info.Uptime = now.Sub(start)
			}
			combinedMem += info.MemBytes
		}
		instances = append(instances, info)
	}

	// Running first, then stopped; SliceStable keeps registry order within groups.
	sort.SliceStable(instances, func(i, j int) bool {
		return instances[i].Running && !instances[j].Running
	})

	return instances, combinedMem, nil
}

// ComposeProjectName resolves the Docker Compose project name for a project.
// Docker projects created by `project create` carry a unique COMPOSE_PROJECT_NAME
// (sw-<name>-<hash>) in .env; older projects fall back to Compose's default of
// the sanitized directory basename.
func ComposeProjectName(projectRoot string) string {
	if name := envfile.ReadComposeProjectName(projectRoot); name != "" {
		return name
	}

	return strings.ToLower(filepath.Base(projectRoot))
}

// memoryByProject sums each running container's memory into its compose project.
// It is best-effort: a docker stats error yields an empty map.
func memoryByProject(ctx context.Context, projectOfContainer map[string]string) map[string]int64 {
	byProject := map[string]int64{}

	out, err := runDocker(ctx, "stats", "--no-stream", "--format", "{{.Name}}\t{{.MemUsage}}")
	if err != nil {
		return byProject
	}

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		name, usage, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		project, ok := projectOfContainer[name]
		if !ok {
			continue
		}
		if b, ok := parseDockerMemUsage(usage); ok {
			byProject[project] += b
		}
	}

	return byProject
}

// earliestStartByProject returns, per compose project, the start time of its
// oldest running container — the project's uptime anchor. Best-effort.
func earliestStartByProject(ctx context.Context, containerIDs []string) map[string]time.Time {
	starts := map[string]time.Time{}
	if len(containerIDs) == 0 {
		return starts
	}

	args := append([]string{"inspect", "--format", "{{index .Config.Labels \"com.docker.compose.project\"}}\t{{.State.StartedAt}}"}, containerIDs...)
	out, err := runDocker(ctx, args...)
	if err != nil {
		return starts
	}

	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		project, ts, ok := strings.Cut(line, "\t")
		if !ok || project == "" {
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(ts))
		if err != nil {
			continue
		}
		if cur, ok := starts[project]; !ok || started.Before(cur) {
			starts[project] = started
		}
	}

	return starts
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
