package pummel

import "strings"

type Comment struct {
	Line  int
	Text  string
	Whole bool
}

type ScanResult struct {
	Masked   string
	Comments []Comment
}

func scan(src string, lang Lang) ScanResult {
	masked := make([]byte, 0, len(src))
	comments := []Comment{}
	line := 1
	sawCode := false
	i := 0
	n := len(src)

	if (lang == LangPHP || lang == LangJSTS) && strings.HasPrefix(src, "#!") {
		idx := strings.IndexByte(src, '\n')
		if idx < 0 {
			idx = n
		}
		for j := 0; j < idx; j++ {
			masked = append(masked, ' ')
		}
		i = idx
	}

	const (
		stCode = iota
		stStr
		stLine
		stBlock
	)
	st := stCode
	var quote byte
	var cb strings.Builder
	cLine, cWhole := 0, false

	for i < n {
		c := src[i]
		switch st {
		case stCode:
			switch {
			case c == '/' && i+1 < n && src[i+1] == '/':
				st = stLine
				cLine, cWhole = line, !sawCode
				cb.Reset()
				masked = append(masked, ' ', ' ')
				i += 2
			case c == '#' && lang == LangPHP:
				st = stLine
				cLine, cWhole = line, !sawCode
				cb.Reset()
				masked = append(masked, ' ')
				i++
			case c == '/' && i+1 < n && src[i+1] == '*':
				st = stBlock
				cLine, cWhole = line, !sawCode
				cb.Reset()
				masked = append(masked, ' ', ' ')
				i += 2
			case c == '"' || c == '\'' || (c == '`' && lang != LangPHP):
				st = stStr
				quote = c
				masked = append(masked, ' ')
				i++
			case c == '\n':
				line++
				sawCode = false
				masked = append(masked, '\n')
				i++
			default:
				if c != ' ' && c != '\t' && c != '\r' {
					sawCode = true
				}
				masked = append(masked, c)
				i++
			}
		case stStr:
			switch {
			case c == '\\' && !(quote == '`' && lang == LangGo) && i+1 < n:
				masked = append(masked, ' ', ' ')
				i += 2
			case c == quote:
				st = stCode
				sawCode = true
				masked = append(masked, ' ')
				i++
			case c == '\n':
				line++
				masked = append(masked, '\n')
				i++
			default:
				masked = append(masked, ' ')
				i++
			}
		case stLine:
			if c == '\n' {
				comments = append(comments, Comment{Line: cLine, Text: strings.TrimSpace(cb.String()), Whole: cWhole})
				st = stCode
				line++
				sawCode = false
				masked = append(masked, '\n')
				i++
			} else {
				cb.WriteByte(c)
				masked = append(masked, ' ')
				i++
			}
		case stBlock:
			switch {
			case c == '*' && i+1 < n && src[i+1] == '/':
				comments = append(comments, Comment{Line: cLine, Text: cleanBlock(cb.String()), Whole: cWhole})
				st = stCode
				sawCode = true
				masked = append(masked, ' ', ' ')
				i += 2
			case c == '\n':
				cb.WriteByte('\n')
				masked = append(masked, '\n')
				line++
				i++
			default:
				cb.WriteByte(c)
				masked = append(masked, ' ')
				i++
			}
		}
	}
	if st == stLine {
		comments = append(comments, Comment{Line: cLine, Text: strings.TrimSpace(cb.String()), Whole: cWhole})
	}
	if st == stBlock {
		comments = append(comments, Comment{Line: cLine, Text: cleanBlock(cb.String()), Whole: cWhole})
	}
	return ScanResult{Masked: string(masked), Comments: comments}
}

func cleanBlock(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		l = strings.TrimSpace(l)
		l = strings.TrimPrefix(l, "*")
		lines[i] = strings.TrimSpace(l)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
