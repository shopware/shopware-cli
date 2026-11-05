package logging

// SilenceError marks errors whose details have already been printed by the
// command. They still cause a non-zero exit, but must not be logged again.
// Implement SilenceError on custom error types to opt in.
type SilenceError interface {
	error
	SilenceError()
}

// NewSilent creates a distinct sentinel error whose details are already printed.
func NewSilent(message string) error {
	return &silentError{message: message}
}

type silentError struct {
	message string
}

func (e *silentError) Error() string { return e.message }
func (e *silentError) SilenceError() {}
