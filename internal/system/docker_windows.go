//go:build windows

package system

func IsDockerMountable() bool {
	return false
}

// ProjectUserSpec returns "" on Windows: containers run via Docker
// Desktop's VM and there is no host UID/GID to map.
func ProjectUserSpec(dir string) string {
	return ""
}

// DockerRunUserArgs returns nil on Windows (no UID mapping needed).
func DockerRunUserArgs(projectDir string) []string {
	return nil
}

func IsDockerUsingLibkrun() bool {
	return false
}
