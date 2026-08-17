package sqlshell

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
)

const (
	promptMain         = "sql> "
	promptContinuation = "  -> "
)

// Shell runs an interactive read-eval-print loop until EOF or an exit
// command (exit, quit, \q). Statement errors are printed and do not end the
// session.
func Shell(ctx context.Context, db Conn, in io.Reader, out, errOut io.Writer, format Format) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var buffer string
	delimiter := DefaultDelimiter

	if _, err := fmt.Fprint(out, promptMain); err != nil {
		return err
	}

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		line := scanner.Text()

		if buffer == "" && isExitCommand(line) {
			return nil
		}

		buffer += line + "\n"

		var statements []string
		// The remainder keeps its trailing newline: the splitter only trims
		// the leading whitespace.
		statements, buffer, delimiter = SplitStatementsWithDelimiter(buffer, delimiter)
		for _, stmt := range statements {
			if err := RunStatement(ctx, db, stmt, out, format); err != nil {
				// Best-effort: the session continues even when stderr is gone.
				_, _ = fmt.Fprintf(errOut, "ERROR: %s\n", err)
			}
		}

		prompt := promptMain
		if buffer != "" {
			prompt = promptContinuation
		}

		if _, err := fmt.Fprint(out, prompt); err != nil {
			return err
		}
	}

	// Execute what is left in the buffer on EOF, so a statement without a
	// trailing semicolon before Ctrl+D is not silently discarded. This
	// matches the trailing statement handling of Run and ExecuteStream.
	if rest := strings.TrimSpace(buffer); rest != "" {
		if err := RunStatement(ctx, db, rest, out, format); err != nil {
			// Best-effort: the session ends anyway.
			_, _ = fmt.Fprintf(errOut, "ERROR: %s\n", err)
		}
	}

	// Cosmetic newline after EOF.
	_, _ = fmt.Fprintln(out)

	return scanner.Err()
}

func isExitCommand(line string) bool {
	switch strings.ToLower(strings.TrimRight(strings.TrimSpace(line), "; \t")) {
	case "exit", "quit", `\q`:
		return true
	}

	return false
}
