package symfony

import "fmt"

// XMLConversionErrorKind identifies a source problem that prevents conversion.
type XMLConversionErrorKind string

const (
	// XMLConversionUnsupported identifies XML that cannot be represented safely.
	XMLConversionUnsupported XMLConversionErrorKind = "unsupported"
	// XMLConversionInvalidXML identifies malformed XML input.
	XMLConversionInvalidXML XMLConversionErrorKind = "invalid-xml"
	// XMLConversionTargetExists identifies a conflicting YAML destination.
	XMLConversionTargetExists XMLConversionErrorKind = "target-exists"
)

// XMLConversionError records the affected XML file, including an imported file.
// Filesystem failures remain ordinary wrapped I/O errors instead.
type XMLConversionError struct {
	Path string
	Kind XMLConversionErrorKind
	Err  error
}

func (e *XMLConversionError) Error() string {
	return fmt.Sprintf("cannot convert %s: %v", e.Path, e.Err)
}

func (e *XMLConversionError) Unwrap() error { return e.Err }
