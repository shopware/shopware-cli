package cmd

import (
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

func applyCommandShortcuts(root *cobra.Command) {
	assignAliases(root)
}

func assignAliases(parent *cobra.Command) {
	children := parent.Commands()
	used := map[string]struct{}{}

	for _, child := range children {
		for _, alias := range child.Aliases {
			used[alias] = struct{}{}
		}
	}

	for _, child := range children {
		for _, candidate := range aliasCandidates(child.Name()) {
			if candidate == "" || candidate == child.Name() {
				continue
			}
			if contains(child.Aliases, candidate) {
				used[candidate] = struct{}{}
				break
			}
			if _, exists := used[candidate]; exists {
				continue
			}

			child.Aliases = append(child.Aliases, candidate)
			used[candidate] = struct{}{}
			break
		}

		assignAliases(child)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}

	return false
}

func aliasCandidates(name string) []string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	if len(parts) == 0 {
		return nil
	}

	compact := strings.Join(parts, "")
	seen := map[string]struct{}{}
	candidates := make([]string, 0, len(compact)+1)

	if len(parts) > 1 {
		initials := make([]byte, 0, len(parts))
		for _, part := range parts {
			initials = append(initials, part[0])
		}
		candidate := string(initials)
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	for i := 1; i <= len(compact); i++ {
		candidate := compact[:i]
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		candidates = append(candidates, candidate)
	}

	return candidates
}
