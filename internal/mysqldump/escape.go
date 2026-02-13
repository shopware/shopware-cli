package mysqldump

import (
	"bytes"
)

var unescapeMap = map[byte]byte{
	'0':  0,
	'n':  '\n',
	'r':  '\r',
	'\\': '\\',
	'\'': '\'',
	'"':  '"',
	'Z':  '\032',
}

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

func unescape(str string) string {
	var buf bytes.Buffer
	for i := 0; i < len(str); i++ {
		if str[i] == '\\' && i+1 < len(str) {
			if unescaped, ok := unescapeMap[str[i+1]]; ok {
				buf.WriteByte(unescaped)
				i++
				continue
			}
		}
		buf.WriteByte(str[i])
	}
	return buf.String()
}
