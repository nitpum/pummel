package pummel

import (
	"fmt"
	"regexp"
	"strings"
)

var toonNumberRe = regexp.MustCompile(`^[+-]?(0|[1-9]\d*|\d+)(\.\d+)?([eE][+-]?\d+)?$`)

func toonNeedsQuote(s string) bool {
	if s == "" {
		return true
	}
	if strings.TrimSpace(s) != s {
		return true
	}
	if s == "true" || s == "false" || s == "null" {
		return true
	}
	if toonNumberRe.MatchString(s) {
		return true
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "#") {
		return true
	}
	if strings.ContainsAny(s, ":\"\\[]{},") {
		return true
	}
	for _, r := range s {
		if r <= 0x1f {
			return true
		}
	}
	return false
}

func toonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r <= 0x1f {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func toonCell(s string) string {
	if toonNeedsQuote(s) {
		return toonQuote(s)
	}
	return s
}

func EncodeFindings(fs []Finding) string {
	if len(fs) == 0 {
		return "[]"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%d]{file,line,rule,severity,message}:\n", len(fs))
	for _, f := range fs {
		cells := []string{toonCell(f.File), fmt.Sprintf("%d", f.Line), toonCell(f.Rule), toonCell(f.Severity), toonCell(f.Message)}
		b.WriteString(strings.Join(cells, ","))
		b.WriteString("\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}
