package tui

import (
	"bufio"
	"io"
	"os/exec"

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

// ReadLineCmd returns a tea.Cmd that reads the next line from ch and converts
// it into a message via onLine, or returns onDone once the channel is closed.
// Re-issue the command after each received line to keep the stream flowing.
func ReadLineCmd(ch <-chan string, onLine func(string) tea.Msg, onDone tea.Msg) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return onDone
		}
		return onLine(line)
	}
}
