package installer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// InstalledNames reads the selected scope from the skills CLI. The remaining
// metadata belongs to skills and is intentionally ignored.
func InstalledNames(ctx context.Context, global bool, stderr io.Writer) (map[string]bool, error) {
	return installedNames(ctx, global, stderr, execute)
}

func installedNames(ctx context.Context, global bool, stderr io.Writer, execute runner) (map[string]bool, error) {
	args := []string{"npx", "--yes", "skills", "list", "--json"}
	if global {
		args = append(args, "--global")
	}
	var output bytes.Buffer
	if err := execute(ctx, "", args, &output, stderr); err != nil {
		return nil, fmt.Errorf("cannot list installed skills: %w", err)
	}
	var entries []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(output.Bytes(), &entries); err != nil {
		return nil, fmt.Errorf("cannot parse installed skills: %w", err)
	}
	names := make(map[string]bool, len(entries))
	for _, entry := range entries {
		names[entry.Name] = true
	}
	return names, nil
}
