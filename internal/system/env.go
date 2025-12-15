package system

import (
	"os"
	"regexp"
)

var envVarRegex = regexp.MustCompile(`\${\w+}`)

func ExpandEnv(s string) string {
	return envVarRegex.ReplaceAllStringFunc(s, func(match string) string {
		return os.Getenv(match[2 : len(match)-1])
	})
}
