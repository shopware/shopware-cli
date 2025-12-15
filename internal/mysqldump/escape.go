package mysqldump

import (
	"bytes"
)

func escape(str string) string {
	var esc string
	var buf bytes.Buffer
	last := 0
	for i, c := range str {
		switch c {
		case 0:
			esc = `\0`
		case '\n':
			esc = `\n`
		case '\r':
			esc = `\r`
		case '\\':
			esc = `\\`
		case '\'':
			esc = `\'`
		case '"':
			esc = `\"`
		case '\032':
			esc = `\Z`
		default:
			continue
		}
		_, _ = buf.WriteString(str[last:i])
		_, _ = buf.WriteString(esc)
		last = i + 1
	}
	_, _ = buf.WriteString(str[last:])
	return buf.String()
}
