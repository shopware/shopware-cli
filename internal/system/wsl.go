package system

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func IsWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}

	data, err := os.ReadFile("/proc/version")
	if err == nil && strings.Contains(strings.ToLower(string(data)), "microsoft") {
		return true
	}

	return os.Getenv("WSL_DISTRO_NAME") != ""
}

func IsWSLWindowsMount(path string) bool {
	if !IsWSL() {
		return false
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	if len(absPath) < 7 {
		return false
	}

	if !strings.HasPrefix(absPath, "/mnt/") {
		return false
	}

	drive := absPath[5]
	return drive >= 'a' && drive <= 'z' && absPath[6] == '/'
}