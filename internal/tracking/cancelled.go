package tracking

// Cancelled marks errors for a run the user chose to stop, which the command event reports as cancelled.
// Implement Cancelled on custom error types to opt in.
type Cancelled interface {
	error
	Cancelled()
}

// NewCancelled creates a distinct sentinel error for a run the user chose to stop.
func NewCancelled(message string) error {
	return &cancelledError{message: message}
}

type cancelledError struct {
	message string
}

func (e *cancelledError) Error() string { return e.message }
func (e *cancelledError) Cancelled()    {}
