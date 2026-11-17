package system

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ValidatePHPCreateMemory checks the effective memory ceiling used by local
// Composer auto-scripts. Flex forwards COMPOSER_MEMORY_LIMIT to their PHP child.
// This does not change PHP configuration or reserve physical memory.
func ValidatePHPCreateMemory(ctx context.Context, phpBinary string) error {
	output, err := phpCreateMemoryCommand(ctx, phpBinary, os.Getenv("COMPOSER_MEMORY_LIMIT")).Output()
	if err != nil {
		return fmt.Errorf("cannot check PHP memory limit using %q: %w", phpBinary, err)
	}
	return validatePHPCreateMemoryOutput(output, phpBinary)
}

func phpCreateMemoryCommand(ctx context.Context, phpBinary, override string) *exec.Cmd {
	var args []string
	if override != "" && override != "0" {
		args = append(args, "-d", "memory_limit="+override)
	}
	args = append(args, "-r", `echo json_encode(['limit' => ini_get('memory_limit'), 'ini' => php_ini_loaded_file() ?: '']);`)
	return exec.CommandContext(ctx, phpBinary, args...)
}

func validatePHPCreateMemoryOutput(output []byte, phpBinary string) error {
	var config struct {
		Limit string `json:"limit"`
		INI   string `json:"ini"`
	}
	if err := json.Unmarshal(output, &config); err != nil {
		return fmt.Errorf("cannot read PHP memory limit using %q: %w", phpBinary, err)
	}
	limit, err := parsePHPMemoryLimit(config.Limit)
	if err != nil {
		return fmt.Errorf("cannot read PHP memory limit using %q: %w", phpBinary, err)
	}
	if limit == -1 || limit >= 512*1024*1024 {
		return nil
	}
	return fmt.Errorf("cannot create a project with PHP memory_limit=%s using %q (php.ini: %q): at least 512M is required; increase memory_limit or re-run with \"COMPOSER_MEMORY_LIMIT=512M\"", config.Limit, phpBinary, config.INI)
}

func parsePHPMemoryLimit(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "-1" {
		return -1, nil
	}
	multiplier := int64(1)
	if len(value) > 0 {
		switch value[len(value)-1] {
		case 'k', 'K':
			multiplier = 1024
		case 'm', 'M':
			multiplier = 1024 * 1024
		case 'g', 'G':
			multiplier = 1024 * 1024 * 1024
		}
		if multiplier != 1 {
			value = strings.TrimSpace(value[:len(value)-1])
		}
	}
	n, err := strconv.ParseInt(value, 0, 64)
	if err != nil || n < 0 || n > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("invalid memory_limit %q", value)
	}
	return n * multiplier, nil
}
