package system

import "os"

func IsInsideContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	if os.Getenv("container") != "" {
		return true
	}

	return false
}
