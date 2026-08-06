package sqlshell

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Execer executes statements that do not return rows.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Conn is the database handle the shell operates on. It is satisfied by
// *sql.Conn (preferred, since session state like SET or USE sticks to a
// single connection) and *sql.DB.
type Conn interface {
	Execer
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Format controls how result sets are rendered.
type Format string

const (
	// FormatTable renders mysql-client style ASCII tables.
	FormatTable Format = "table"
	// FormatTSV renders tab-separated values with a header line.
	FormatTSV Format = "tsv"
	// FormatJSON renders each result set as a JSON array of row objects.
	FormatJSON Format = "json"
)

// ParseFormat validates a format name given on the command line.
func ParseFormat(name string) (Format, error) {
	switch Format(name) {
	case FormatTable, FormatTSV, FormatJSON:
		return Format(name), nil
	}

	return "", fmt.Errorf("unknown format %q, allowed values: table, tsv, json", name)
}

// Run executes all statements in input against db and renders the results to out.
func Run(ctx context.Context, db Conn, input string, out io.Writer, format Format) error {
	statements, rest, _ := SplitStatementsWithDelimiter(input, DefaultDelimiter)
	if rest != "" {
		// The trailing semicolon is optional.
		statements = append(statements, rest)
	}

	multiple := len(statements) > 1

	for _, stmt := range statements {
		if err := RunStatement(ctx, db, stmt, out, format); err != nil {
			if multiple {
				return fmt.Errorf("%q: %w", summarizeStatement(stmt), err)
			}

			return err
		}
	}

	return nil
}

// RunStatement executes a single statement and renders its result to out.
// Comment-only statements and stray DELIMITER directives (client commands,
// not SQL) are skipped.
func RunStatement(ctx context.Context, db Conn, stmt string, out io.Writer, format Format) error {
	keyword := firstKeyword(stmt)
	if keyword == "" || keyword == "delimiter" {
		return nil
	}

	start := time.Now()

	if !returnsResultSet(keyword) && !hasReturningClause(keyword, stmt) {
		result, err := db.ExecContext(ctx, stmt)
		if err != nil {
			return err
		}

		affected, err := result.RowsAffected()
		if err != nil {
			affected = 0
		}

		if format == FormatJSON {
			_, err := fmt.Fprintf(out, "{\"rows_affected\":%d}\n", affected)
			return err
		}

		_, err = fmt.Fprintf(out, "Query OK, %d %s affected (%s)\n", affected, pluralRows(affected), formatDuration(time.Since(start)))

		return err
	}

	rows, err := db.QueryContext(ctx, stmt)
	if err != nil {
		return err
	}
	// Read errors surface through rows.Err below, the close error is redundant.
	defer func() { _ = rows.Close() }()

	// A statement like CALL can produce more than one result set.
	for {
		if err := renderResultSet(rows, time.Since(start), out, format); err != nil {
			return err
		}

		if !rows.NextResultSet() {
			break
		}
	}

	return rows.Err()
}

// firstKeyword returns the lowercased first word of the statement, skipping
// leading comments and parentheses. MySQL executable comments (/*!40014 ...)
// count as statement content. It returns "" for comment-only input.
func firstKeyword(stmt string) string {
	for {
		stmt = strings.TrimLeft(stmt, " \t\r\n(")

		switch {
		case strings.HasPrefix(stmt, "/*!"):
			// Executable comment: the server runs its content, so the
			// keyword follows the marker and optional version number.
			stmt = strings.TrimLeft(stmt[3:], "0123456789")
		case strings.HasPrefix(stmt, "/*"):
			_, after, found := strings.Cut(stmt[2:], "*/")
			if !found {
				return ""
			}
			stmt = after
		case strings.HasPrefix(stmt, "#"), strings.HasPrefix(stmt, "--"):
			_, after, found := strings.Cut(stmt, "\n")
			if !found {
				return ""
			}
			stmt = after
		default:
			end := strings.IndexFunc(stmt, func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '(' || r == ';'
			})
			if end == -1 {
				return strings.ToLower(stmt)
			}

			return strings.ToLower(stmt[:end])
		}
	}
}

// returnsResultSet reports whether a statement starting with keyword is
// expected to return rows and should go through Query instead of Exec.
func returnsResultSet(keyword string) bool {
	switch keyword {
	case "select", "show", "describe", "desc", "explain", "with", "values", "table", "call", "check", "checksum", "analyze", "optimize", "repair", "help":
		return true
	}

	return false
}

// hasReturningClause reports whether a DML statement carries a MariaDB
// RETURNING clause and therefore produces a result set. The keyword scan
// ignores quoted sections and comments, follows MariaDB's rule that "--" is
// only a comment when followed by whitespace, and treats completed comments
// as token separators.
func hasReturningClause(keyword, stmt string) bool {
	switch keyword {
	case "insert", "replace", "delete":
	default:
		return false
	}

	const returning = "RETURNING"

	state := stateNormal
	// boundary tracks whether the previous position separates tokens, so the
	// keyword is not matched inside identifiers like returning_col.
	boundary := true

	for i := 0; i < len(stmt); i++ {
		if state != stateNormal {
			before := state
			state, i = consumeQuoteOrComment(state, stmt, i)

			if state == stateNormal {
				// A closed comment separates tokens, a closed literal does not.
				boundary = before == stateLineComment || before == stateBlockComment
			}

			continue
		}

		c := stmt[i]

		switch {
		case c == '\'':
			state = stateSingleQuote
			boundary = false
		case c == '"':
			state = stateDoubleQuote
			boundary = false
		case c == '`':
			state = stateBacktick
			boundary = false
		case c == '#':
			state = stateLineComment
		case c == '-' && strings.HasPrefix(stmt[i:], "--") && (i+2 >= len(stmt) || isSQLSpace(stmt[i+2])):
			state = stateLineComment
			i++
		case c == '/' && strings.HasPrefix(stmt[i:], "/*"):
			state = stateBlockComment
			i++
		case isSQLSpace(c) || c == '(' || c == ')' || c == ',':
			boundary = true
		case (c == 'r' || c == 'R') && boundary:
			if i+len(returning) <= len(stmt) && strings.EqualFold(stmt[i:i+len(returning)], returning) && boundaryAfterKeyword(stmt, i+len(returning)) {
				return true
			}

			boundary = false
		default:
			boundary = false
		}
	}

	return false
}

func isSQLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// boundaryAfterKeyword reports whether the keyword ending right before pos is
// followed by a token separator: end of input, whitespace, punctuation or a
// block comment.
func boundaryAfterKeyword(stmt string, pos int) bool {
	if pos >= len(stmt) {
		return true
	}

	if isSQLSpace(stmt[pos]) || stmt[pos] == '(' || stmt[pos] == '*' {
		return true
	}

	return strings.HasPrefix(stmt[pos:], "/*")
}

func summarizeStatement(stmt string) string {
	stmt = strings.Join(strings.Fields(stmt), " ")

	const maxLen = 60
	if utf8.RuneCountInString(stmt) > maxLen {
		return string([]rune(stmt)[:maxLen]) + "…"
	}

	return stmt
}

func renderResultSet(rows *sql.Rows, elapsed time.Duration, out io.Writer, format Format) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	// Non-query statements executed through Query (e.g. the OK packet at the
	// end of a CALL) produce a result set without columns.
	if len(columns) == 0 {
		return nil
	}

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return err
	}

	binary := make([]bool, len(columnTypes))
	numeric := make([]bool, len(columnTypes))
	for i, columnType := range columnTypes {
		binary[i] = isBinaryType(columnType.DatabaseTypeName())
		numeric[i] = isNumericType(columnType.DatabaseTypeName())
	}

	var data [][]any

	values := make([]sql.RawBytes, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return err
		}

		row := make([]any, len(columns))
		for i, value := range values {
			switch {
			case value == nil:
				row[i] = nil
			case binary[i]:
				row[i] = "0x" + strings.ToUpper(hex.EncodeToString(value))
			default:
				row[i] = string(value)
			}
		}

		data = append(data, row)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	if format == FormatJSON {
		return renderJSON(columns, numeric, data, out)
	}

	if format == FormatTSV {
		return renderTSV(columns, data, out)
	}

	return renderTable(columns, data, elapsed, out)
}

func isBinaryType(databaseType string) bool {
	switch strings.ToUpper(databaseType) {
	case "BINARY", "VARBINARY", "BLOB", "TINYBLOB", "MEDIUMBLOB", "LONGBLOB", "BIT", "GEOMETRY":
		return true
	}

	return false
}

func isNumericType(databaseType string) bool {
	switch strings.ToUpper(databaseType) {
	case "TINYINT", "SMALLINT", "MEDIUMINT", "INT", "BIGINT", "UNSIGNED TINYINT", "UNSIGNED SMALLINT", "UNSIGNED MEDIUMINT", "UNSIGNED INT", "UNSIGNED BIGINT", "DECIMAL", "FLOAT", "DOUBLE", "YEAR":
		return true
	}

	return false
}

func renderTable(columns []string, data [][]any, elapsed time.Duration, out io.Writer) error {
	widths := make([]int, len(columns))
	for i, column := range columns {
		widths[i] = utf8.RuneCountInString(column)
	}

	cells := make([][]string, len(data))
	for r, row := range data {
		cells[r] = make([]string, len(columns))
		for i, value := range row {
			cell := "NULL"
			if value != nil {
				cell = value.(string)
			}

			cells[r][i] = cell
			if width := utf8.RuneCountInString(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}

	var builder strings.Builder

	writeSeparator := func() {
		for _, width := range widths {
			builder.WriteString("+")
			builder.WriteString(strings.Repeat("-", width+2))
		}
		builder.WriteString("+\n")
	}

	writeRow := func(row []string) {
		for i, cell := range row {
			builder.WriteString("| ")
			builder.WriteString(cell)
			builder.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+1))
		}
		builder.WriteString("|\n")
	}

	if len(data) == 0 {
		_, err := fmt.Fprintf(out, "Empty set (%s)\n", formatDuration(elapsed))
		return err
	}

	writeSeparator()
	writeRow(columns)
	writeSeparator()
	for _, row := range cells {
		writeRow(row)
	}
	writeSeparator()

	if _, err := io.WriteString(out, builder.String()); err != nil {
		return err
	}

	_, err := fmt.Fprintf(out, "%d %s in set (%s)\n", len(data), pluralRows(int64(len(data))), formatDuration(elapsed))

	return err
}

func pluralRows(count int64) string {
	if count == 1 {
		return "row"
	}

	return "rows"
}

func renderTSV(columns []string, data [][]any, out io.Writer) error {
	escape := strings.NewReplacer("\\", "\\\\", "\t", "\\t", "\n", "\\n", "\r", "\\r")

	writeLine := func(cells []string) error {
		_, err := fmt.Fprintln(out, strings.Join(cells, "\t"))
		return err
	}

	header := make([]string, len(columns))
	for i, column := range columns {
		header[i] = escape.Replace(column)
	}

	if err := writeLine(header); err != nil {
		return err
	}

	for _, row := range data {
		cells := make([]string, len(row))
		for i, value := range row {
			if value == nil {
				cells[i] = "NULL"
				continue
			}

			cells[i] = escape.Replace(value.(string))
		}

		if err := writeLine(cells); err != nil {
			return err
		}
	}

	return nil
}

func renderJSON(columns []string, numeric []bool, data [][]any, out io.Writer) error {
	result := make([]map[string]any, len(data))

	for r, row := range data {
		object := make(map[string]any, len(columns))
		for i, column := range columns {
			value := row[i]

			if str, ok := value.(string); ok && numeric[i] && json.Valid([]byte(str)) {
				value = json.RawMessage(str)
			}

			object[column] = value
		}

		result[r] = object
	}

	encoder := json.NewEncoder(out)

	return encoder.Encode(result)
}

func formatDuration(elapsed time.Duration) string {
	return fmt.Sprintf("%.3f sec", elapsed.Seconds())
}
