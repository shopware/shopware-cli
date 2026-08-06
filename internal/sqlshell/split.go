// Package sqlshell implements a small SQL shell used by `project sql` and
// `project restore`: splitting scripts into statements, executing them and
// rendering results.
package sqlshell

import "strings"

// DefaultDelimiter is the statement terminator used unless a DELIMITER
// directive changes it.
const DefaultDelimiter = ";"

const (
	stateNormal = iota
	stateSingleQuote
	stateDoubleQuote
	stateBacktick
	stateLineComment
	stateBlockComment
)

// SplitStatements splits input into complete SQL statements and returns the
// trailing incomplete remainder. It starts with the default ";" delimiter;
// use SplitStatementsWithDelimiter to carry delimiter state across calls.
func SplitStatements(input string) ([]string, string) {
	statements, rest, _ := SplitStatementsWithDelimiter(input, DefaultDelimiter)
	return statements, rest
}

// SplitStatementsWithDelimiter splits input into complete SQL statements,
// terminated by delimiter, and returns the trailing incomplete remainder plus
// the delimiter active after the input. Terminators inside single-quoted,
// double-quoted or backtick-quoted sections, line comments (-- , #) and block
// comments (/* */) do not end a statement.
//
// DELIMITER directives (a client feature used by dumps around triggers and
// routines) are consumed, not returned as statements: they change the active
// delimiter from their line onwards.
//
// Only leading whitespace is trimmed from the remainder: its trailing
// whitespace is significant when more input is appended later (streaming).
func SplitStatementsWithDelimiter(input, delimiter string) ([]string, string, string) {
	if delimiter == "" {
		delimiter = DefaultDelimiter
	}

	var statements []string

	state := stateNormal
	start := 0
	// blankStatement tracks whether only whitespace and comments were seen
	// since the last statement boundary; a DELIMITER directive is only
	// recognized there.
	blankStatement := true

	for i := 0; i < len(input); i++ {
		c := input[i]

		if state != stateNormal {
			state, i = consumeQuoteOrComment(state, input, i)
			continue
		}

		if blankStatement && isDelimiterDirective(input[i:]) {
			lineEnd := strings.IndexByte(input[i:], '\n')
			if lineEnd == -1 {
				// The directive line may continue in the next chunk.
				return statements, strings.TrimLeft(input[i:], " \t\r\n"), delimiter
			}

			if token := delimiterToken(input[i : i+lineEnd]); token != "" {
				delimiter = token
			}

			start = i + lineEnd + 1
			i += lineEnd

			continue
		}

		if strings.HasPrefix(input[i:], delimiter) {
			if stmt := strings.TrimSpace(input[start:i]); stmt != "" {
				statements = append(statements, stmt)
			}

			start = i + len(delimiter)
			i = start - 1
			blankStatement = true

			continue
		}

		switch c {
		case '\'':
			state = stateSingleQuote
			blankStatement = false
		case '"':
			state = stateDoubleQuote
			blankStatement = false
		case '`':
			state = stateBacktick
			blankStatement = false
		case '#':
			state = stateLineComment
		case '-':
			// MySQL only treats "--" as a comment when followed by
			// whitespace or the end of the line.
			if strings.HasPrefix(input[i:], "--") && (i+2 >= len(input) || input[i+2] == ' ' || input[i+2] == '\t' || input[i+2] == '\n' || input[i+2] == '\r') {
				state = stateLineComment
				i++
			} else {
				blankStatement = false
			}
		case '/':
			if strings.HasPrefix(input[i:], "/*") {
				state = stateBlockComment
				i++
			} else {
				blankStatement = false
			}
		case ' ', '\t', '\r', '\n':
			// whitespace keeps the statement blank
		default:
			blankStatement = false
		}
	}

	return statements, strings.TrimLeft(input[start:], " \t\r\n"), delimiter
}

// consumeQuoteOrComment advances the scanner while inside a quoted section
// or comment. It processes input[i] and returns the resulting state and the
// last consumed index.
func consumeQuoteOrComment(state int, input string, i int) (int, int) {
	c := input[i]

	switch state {
	case stateSingleQuote:
		switch c {
		case '\\':
			i++
		case '\'':
			state = stateNormal
		}
	case stateDoubleQuote:
		switch c {
		case '\\':
			i++
		case '"':
			state = stateNormal
		}
	case stateBacktick:
		if c == '`' {
			state = stateNormal
		}
	case stateLineComment:
		if c == '\n' {
			state = stateNormal
		}
	case stateBlockComment:
		if strings.HasPrefix(input[i:], "*/") {
			state = stateNormal
			i++
		}
	}

	return state, i
}

// isDelimiterDirective reports whether s starts with a DELIMITER keyword
// followed by its argument.
func isDelimiterDirective(s string) bool {
	const keyword = "delimiter"

	if len(s) <= len(keyword) || !strings.EqualFold(s[:len(keyword)], keyword) {
		return false
	}

	return s[len(keyword)] == ' ' || s[len(keyword)] == '\t'
}

// delimiterToken extracts the new delimiter from a DELIMITER directive line.
func delimiterToken(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ""
	}

	return fields[1]
}
