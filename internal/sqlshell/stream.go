package sqlshell

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// ExecuteStream reads SQL statements from r and executes them one by one,
// without rendering results. It is meant for restoring dumps, where the input
// can be far larger than memory. onStatement, when non-nil, is called after
// every executed statement with the running count. The number of executed
// statements is returned, also when an error aborts the run.
func ExecuteStream(ctx context.Context, db Execer, r io.Reader, onStatement func(count int)) (int, error) {
	// No bufio wrapper: reads of len(chunk) would bypass its internal buffer
	// anyway, it would only allocate a second unused megabyte.
	chunk := make([]byte, 1<<20)

	var buffer string
	delimiter := DefaultDelimiter
	count := 0

	execute := func(stmt string) error {
		if keyword := firstKeyword(stmt); keyword == "" || keyword == "delimiter" {
			return nil
		}

		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%q: %w", summarizeStatement(stmt), err)
		}

		count++

		if onStatement != nil {
			onStatement(count)
		}

		return nil
	}

	for {
		n, readErr := r.Read(chunk)

		if n > 0 {
			buffer += string(chunk[:n])

			var statements []string
			statements, buffer, delimiter = SplitStatementsWithDelimiter(buffer, delimiter)
			for _, stmt := range statements {
				if err := execute(stmt); err != nil {
					return count, err
				}
			}
		}

		if readErr == io.EOF {
			break
		}

		if readErr != nil {
			return count, readErr
		}
	}

	// The trailing semicolon is optional.
	if strings.TrimSpace(buffer) != "" {
		if err := execute(buffer); err != nil {
			return count, err
		}
	}

	return count, nil
}
