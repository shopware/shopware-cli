package validation

import (
	"path/filepath"
	"slices"
	"strings"
)

const darwinPrivatePrefix = "/private"

// ResolveSourceRoot returns a cleaned, absolute source root used to rewrite
// finding locations. Temporary extraction directories stay usable as roots;
// only the reported path is rewritten to be relative to this root.
func ResolveSourceRoot(root string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}

	cleaned := filepath.Clean(root)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return cleaned
	}

	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}

	return abs
}

// NormalizeSourcePath rewrites a finding path so it is relative to root.
// Absolute workspace, extraction, and macOS /private prefixes are stripped.
// Paths that already are relative are returned as slash-normalized relatives.
func NormalizeSourcePath(source, root string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}

	if root == "" {
		if filepath.IsAbs(source) {
			return filepath.ToSlash(filepath.Clean(source))
		}
		return filepath.ToSlash(filepath.Clean(source))
	}

	for _, candidate := range pathVariants(source) {
		for _, rootVariant := range pathVariants(root) {
			if rel, ok := relInside(rootVariant, candidate); ok {
				return rel
			}
		}
	}

	if !filepath.IsAbs(source) {
		return filepath.ToSlash(filepath.Clean(source))
	}

	return filepath.ToSlash(filepath.Clean(source))
}

// StripRootFromText removes temporary or analysis-root prefixes from messages
// so reporter output never leaks the verifier workspace.
func StripRootFromText(text, root string) string {
	if text == "" || root == "" {
		return text
	}

	variants := pathVariants(root)
	slices.SortFunc(variants, func(a, b string) int {
		return len(b) - len(a)
	})

	for _, variant := range variants {
		if variant == "" || variant == "/" || variant == "." {
			continue
		}

		slashVariant := filepath.ToSlash(variant)
		text = replaceRootPrefix(text, slashVariant)
		if slashVariant != variant {
			text = replaceRootPrefix(text, variant)
		}
	}

	return text
}

// NormalizeResult rewrites path, message, and a missing line so every reporter
// sees the same stable source location.
func NormalizeResult(result CheckResult, root string) CheckResult {
	result.Path = NormalizeSourcePath(result.Path, root)
	result.Message = StripRootFromText(result.Message, root)
	if result.Line <= 0 && result.Path != "" {
		result.Line = 1
	}
	return result
}

func replaceRootPrefix(text, root string) string {
	if root == "" {
		return text
	}

	separators := []string{"/", string(filepath.Separator)}
	for _, sep := range separators {
		text = strings.ReplaceAll(text, root+sep, "")
	}

	return replaceExactRootToken(text, root)
}

func replaceExactRootToken(text, root string) string {
	if text == root {
		return "."
	}

	var b strings.Builder
	start := 0
	for {
		idx := strings.Index(text[start:], root)
		if idx < 0 {
			b.WriteString(text[start:])
			return b.String()
		}
		idx += start
		after := idx + len(root)
		if isRootBoundary(text, idx, after) {
			b.WriteString(text[start:idx])
			b.WriteString(".")
			start = after
			continue
		}
		b.WriteString(text[start : idx+1])
		start = idx + 1
	}
}

func isRootBoundary(text string, start, after int) bool {
	if start > 0 {
		prev := text[start-1]
		if prev != ' ' && prev != '\t' && prev != '\n' && prev != '"' && prev != '\'' {
			return false
		}
	}
	if after >= len(text) {
		return true
	}
	next := text[after]
	return next == ' ' || next == '\t' || next == '\n' || next == '"' || next == '\'' || next == ',' || next == '.'
}

func relInside(root, source string) (string, bool) {
	rel, err := filepath.Rel(root, source)
	if err != nil {
		return "", false
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	if rel == "." {
		return ".", true
	}

	return filepath.ToSlash(rel), true
}

func pathVariants(p string) []string {
	if p == "" {
		return nil
	}

	seen := make(map[string]struct{})
	var out []string
	add := func(value string) {
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}

	cleaned := filepath.Clean(p)
	add(cleaned)
	add(filepath.ToSlash(cleaned))

	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		add(resolved)
		add(filepath.ToSlash(resolved))
	}

	for _, base := range slices.Clone(out) {
		add(stripDarwinPrivate(base))
		add(withDarwinPrivate(base))
	}

	return out
}

func stripDarwinPrivate(p string) string {
	slashP := filepath.ToSlash(p)
	if strings.HasPrefix(slashP, darwinPrivatePrefix+"/") {
		return strings.TrimPrefix(slashP, darwinPrivatePrefix)
	}
	return p
}

func withDarwinPrivate(p string) string {
	slashP := filepath.ToSlash(p)
	if strings.HasPrefix(slashP, darwinPrivatePrefix+"/") || !strings.HasPrefix(slashP, "/") {
		return p
	}
	return darwinPrivatePrefix + slashP
}
