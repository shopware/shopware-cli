//go:build windows

package system

func IsDockerMountable() bool {
	return false
}

func IsDockerUsingLibkrun() bool {
	return false
}
