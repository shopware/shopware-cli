package local

import (
	"errors"
	"io"
	"sync"
)

func writerOrDiscard(writer io.Writer) io.Writer {
	if writer == nil {
		return io.Discard
	}
	return writer
}

type outputWrites struct {
	// mu serializes both output streams and protects their write errors.
	mu  sync.Mutex
	err error
}

type trackedWriter struct {
	writer io.Writer
	state  *outputWrites
}

// Write serializes forwarding and records errors, including short writes.
func (w trackedWriter) Write(data []byte) (int, error) {
	w.state.mu.Lock()
	defer w.state.mu.Unlock()

	n, err := w.writer.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		if w.state.err == nil {
			w.state.err = err
		} else {
			w.state.err = errors.Join(w.state.err, err)
		}
	}

	return n, err
}
