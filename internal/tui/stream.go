package tui

import (
	"bufio"
	"bytes"
	"io"
	"os/exec"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// StreamBufferSize is the channel buffer used for streaming subprocess output
// lines into a Bubble Tea program.
const StreamBufferSize = 50

// StreamCmdOutput starts cmd and streams its output line by line into ch,
// closing the channel when the command finishes. With useStdout true, stderr
// is merged into stdout; otherwise stdout is merged into stderr. It blocks
// until the command exits and returns its error.
func StreamCmdOutput(cmd *exec.Cmd, ch chan<- string, useStdout bool) error {
	var pipe io.Reader
	var err error
	if useStdout {
		pipe, err = cmd.StdoutPipe()
		if err == nil {
			cmd.Stderr = cmd.Stdout
		}
	} else {
		pipe, err = cmd.StderrPipe()
		if err == nil {
			cmd.Stdout = cmd.Stderr
		}
	}
	if err != nil {
		close(ch)
		return err
	}

	if err := cmd.Start(); err != nil {
		close(ch)
		return err
	}

	scanner := bufio.NewScanner(pipe)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		ch <- scanner.Text()
	}
	close(ch)

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}

// LineWriter converts a byte stream into per-line emit calls — the io.Writer
// side of streaming subprocess output.
type LineWriter struct {
	mu   sync.Mutex
	emit func(string)
	buf  []byte
}

// NewLineWriter creates a line writer that calls emit for every full line.
func NewLineWriter(emit func(string)) *LineWriter {
	return &LineWriter{emit: emit}
}

// Write implements io.Writer.
func (w *LineWriter) Write(p []byte) (int, error) {
	var lines []string

	w.mu.Lock()
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		lines = append(lines, string(bytes.TrimRight(w.buf[:idx], "\r")))
		w.buf = w.buf[idx+1:]
	}
	w.mu.Unlock()

	// Emit outside the lock: a blocking emit (channel backpressure) must not
	// hold up concurrent writers or Flush.
	for _, line := range lines {
		w.emit(line)
	}
	return len(p), nil
}

// Flush emits any trailing output that did not end in a newline.
func (w *LineWriter) Flush() {
	w.mu.Lock()
	rest := string(w.buf)
	w.buf = nil
	w.mu.Unlock()

	if rest != "" {
		w.emit(rest)
	}
}

// ReadLineCmd returns a tea.Cmd that reads the next line from ch and converts
// it into a message via onLine, or returns onDone once the channel is closed.
// Re-issue the command after each received line to keep the stream flowing.
func ReadLineCmd(ch <-chan string, onLine func(string) tea.Msg, onDone tea.Msg) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return onDone
		}
		line, ok := <-ch
		if !ok {
			return onDone
		}
		return onLine(line)
	}
}
