package validation

import "strings"

// IgnoreMatches reports whether a result should be removed for the given ignore
// rule. Path matching is delegated so callers can handle path normalization.
func IgnoreMatches(result CheckResult, ignore ToolConfigIgnore, pathMatches func(resultPath, ignorePath string) bool) bool {
	// Only ignore all matches when identifier is the only field specified
	if ignore.Identifier != "" && ignore.Path == "" && ignore.Message == "" {
		return IdentifierMatches(result.Identifier, ignore.Identifier)
	}

	// If path is specified with identifier (but no message), match both
	if ignore.Identifier != "" && ignore.Path != "" && ignore.Message == "" {
		return IdentifierMatches(result.Identifier, ignore.Identifier) && pathMatches(result.Path, ignore.Path)
	}

	// If identifier and message are specified (path is optional), match all specified fields
	if ignore.Identifier != "" && ignore.Message != "" {
		return IdentifierMatches(result.Identifier, ignore.Identifier) &&
			strings.Contains(result.Message, ignore.Message) &&
			(ignore.Path == "" || pathMatches(result.Path, ignore.Path))
	}

	// Handle message-based ignores (when no identifier is specified)
	if ignore.Identifier == "" && ignore.Message != "" {
		return strings.Contains(result.Message, ignore.Message) && (pathMatches(result.Path, ignore.Path) || ignore.Path == "")
	}

	// Handle path-only ignores
	if ignore.Identifier == "" && ignore.Message == "" && ignore.Path != "" {
		return pathMatches(result.Path, ignore.Path)
	}

	return false
}
