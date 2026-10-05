package validation

import "strings"

// IdentifierMatches reports whether a result identifier is covered by an ignore
// identifier. An ignore of "metadata.description" matches that exact identifier
// and any more specific child such as "metadata.description.length.de-DE".
func IdentifierMatches(resultIdentifier, ignoreIdentifier string) bool {
	if ignoreIdentifier == "" {
		return false
	}
	if resultIdentifier == ignoreIdentifier {
		return true
	}

	return strings.HasPrefix(resultIdentifier, ignoreIdentifier+".")
}
