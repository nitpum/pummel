package pummel

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxCommentLen = 120
const minCommentedOutWords = 3

type Finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type Source struct {
	Path          string
	ActualPath    string
	Lang          Lang
	Comments      []Comment
	MaskedLines   []string
	RawLines      []string
	IsTest        bool
	AST           *AST
	importAliases func() aliases
}

func (s *Source) Close() {
	s.AST.Close()
}

type Rule struct {
	ID          string
	Description string
	Langs       string
	Fn          func(*Source) []Finding
}

func Load(path string, content []byte, isTest bool) *Source {
	return LoadFile(path, path, content, isTest)
}

func LoadFile(path, actualPath string, content []byte, isTest bool) *Source {
	lang := DetectLang(path)
	s := &Source{
		Path: path, ActualPath: actualPath, Lang: lang, IsTest: isTest,
		AST:           ParseAST(path, lang, content),
		importAliases: sync.OnceValue(func() aliases { return findAliases(actualPath) }),
	}
	if lang == LangJSTS && s.AST != nil {
		s.Comments = astJSComments(s)
	} else {
		res := scan(string(content), lang)
		s.Comments = res.Comments
		s.MaskedLines = strings.Split(res.Masked, "\n")
		s.RawLines = strings.Split(string(content), "\n")
	}
	return s
}

var AllRules = []Rule{
	{ID: "comment-prefix", Description: "every comment must start with TODO:, NOTE:, or FIXME:", Langs: "js/ts, go, php", Fn: commentPrefixRule},
	{ID: "ticket-ref", Description: "no ticket/PR/MR IDs in comments (allowed only inside TODO:/FIXME:)", Langs: "js/ts, go, php", Fn: ticketRefRule},
	{ID: "comment-length", Description: "comments stay short (<= 120 chars)", Langs: "js/ts, go, php", Fn: commentLengthRule},
	{ID: "commented-out-code", Description: "no commented-out code - git keeps the history", Langs: "js/ts, go, php", Fn: commentedOutRule},
	{ID: "verb-first", Description: "function names start with a verb (fetchUserData, not userInfo)", Langs: "js/ts, go, php", Fn: func(s *Source) []Finding {
		if s.AST != nil {
			return astVerbFirst(s)
		}
		return regexVerbFirst(s)
	}},
	{ID: "magic-number", Description: "name numeric literals (exempts named config values and test support data)", Langs: "js/ts, go, php", Fn: magicNumberRule},
	{ID: "relative-import", Description: "prefer a configured alias over ../ imports", Langs: "js/ts", Fn: func(s *Source) []Finding {
		if s.AST != nil {
			return astRelativeImport(s)
		}
		return regexRelativeImport(s)
	}},
}

func Run(src *Source, disabled map[string]bool) []Finding {
	var out []Finding
	for _, r := range AllRules {
		if disabled[r.ID] {
			continue
		}
		out = append(out, r.Fn(src)...)
	}
	return out
}

func mk(path string, line int, rule, severity, msg string) Finding {
	return Finding{File: path, Line: line, Rule: rule, Severity: severity, Message: msg}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

var symbolOnly = regexp.MustCompile(`^[-=*_/#~\s]*$`)

func allowedPrefix(t string) (todo, note, fixme bool) {
	u := strings.ToUpper(t)
	switch {
	case strings.HasPrefix(u, "TODO:"):
		return true, false, false
	case strings.HasPrefix(u, "NOTE:"):
		return false, true, false
	case strings.HasPrefix(u, "FIXME:"):
		return false, false, true
	}
	return false, false, false
}

func commentPrefixRule(s *Source) []Finding {
	var out []Finding
	for _, c := range s.Comments {
		t := firstLine(c.Text)
		if t == "" || symbolOnly.MatchString(t) {
			continue
		}
		if s.Lang == LangGo && strings.HasPrefix(c.Text, "go:") {
			continue
		}
		todo, note, fixme := allowedPrefix(t)
		if !todo && !note && !fixme {
			out = append(out, mk(s.Path, c.Line, "comment-prefix", "warn",
				"comment must start with TODO:, NOTE:, or FIXME: — or make the code self-explanatory"))
		}
	}
	return out
}

var (
	ticketRe    = regexp.MustCompile(`\b([A-Z][A-Z0-9]{1,9})-(\d+)\b`)
	ticketWord  = regexp.MustCompile(`\b(?:PR|MR|CL)[ -]?\d+\b`)
	issueRe     = regexp.MustCompile(`#\d+`)
	urlRe       = regexp.MustCompile(`https?://\S+`)
	urlTicketRe = regexp.MustCompile(`https?://\S*(?:/pull/|/merge_requests/|/issues/)\S*`)
)

var techAcronyms = map[string]bool{
	"UTF": true, "SHA": true, "AES": true, "RSA": true, "DES": true, "MD": true,
	"ISO": true, "HTTP": true, "HTTPS": true, "TLS": true, "SSL": true, "IP": true,
	"CPU": true, "GPU": true, "RAM": true, "ROM": true, "CDN": true, "DNS": true,
	"ACL": true, "JWT": true, "ASCII": true, "ANSI": true, "HKDF": true, "ECDSA": true,
	"PKCS": true, "OIDC": true, "REST": true, "GRAPHQL": true, "CRON": true,
}

func ticketRefRule(s *Source) []Finding {
	var out []Finding
	for _, c := range s.Comments {
		todo, _, fixme := allowedPrefix(firstLine(c.Text))
		if todo || fixme {
			continue
		}
		hit := ""
		if u := urlTicketRe.FindString(c.Text); u != "" {
			hit = u
		} else {
			body := urlRe.ReplaceAllString(c.Text, " ")
			if m := ticketRe.FindStringSubmatch(body); m != nil && !techAcronyms[m[1]] {
				hit = m[0]
			} else if w := ticketWord.FindString(body); w != "" {
				hit = w
			} else if i := issueRe.FindString(body); i != "" {
				hit = i
			}
		}
		if hit != "" {
			out = append(out, mk(s.Path, c.Line, "ticket-ref", "warn",
				fmt.Sprintf("ticket/PR reference %q in comment — only TODO:/FIXME: may reference tickets", hit)))
		}
	}
	return out
}

func commentLengthRule(s *Source) []Finding {
	var out []Finding
	for _, c := range s.Comments {
		if n := utf8.RuneCountInString(c.Text); n > maxCommentLen {
			out = append(out, mk(s.Path, c.Line, "comment-length", "info",
				fmt.Sprintf("comment is %d chars — keep comments short", n)))
		}
	}
	return out
}

var (
	coFirstWord  = regexp.MustCompile(`^(func|function|return|defer|package|import|var|let|const|type|class|interface|enum|switch|case|default|break|continue|if|else|elseif|foreach|for|while|do|try|catch|finally|throw|new|delete|echo|print|public|private|protected|static|void|await|async|yield|struct|chan|goto|namespace|use|extends|implements)\b`)
	coStructural = regexp.MustCompile(`[(={;:]`)
	coCall       = regexp.MustCompile(`^[\w$.]+\([^;]*\)\s*;?$`)
)

func commentedOutRule(s *Source) []Finding {
	var out []Finding
	for _, c := range s.Comments {
		if !c.Whole {
			continue
		}
		if todo, note, fixme := allowedPrefix(firstLine(c.Text)); todo || note || fixme {
			continue
		}
		body := urlRe.ReplaceAllString(c.Text, " ")
		first := strings.TrimSpace(firstLine(body))
		if first == "" || symbolOnly.MatchString(first) {
			continue
		}
		flag := coCall.MatchString(first) || strings.Contains(body, ":=")
		if !flag && coFirstWord.MatchString(first) {
			word := first
			if i := strings.IndexAny(word, " \t"); i > 0 {
				word = word[:i]
			}
			structural := coStructural.MatchString(body)
			alwaysFlag := map[string]bool{
				"func": true, "function": true, "defer": true, "package": true, "import": true,
				"var": true, "let": true, "const": true, "type": true, "class": true,
				"interface": true, "enum": true, "struct": true, "public": true, "private": true,
				"protected": true, "static": true, "echo": true, "print": true, "namespace": true,
				"use": true, "extends": true, "implements": true, "chan": true, "goto": true,
			}
			if alwaysFlag[word] || structural {
				flag = true
			}
		}
		if !flag {
			trimmed := strings.TrimRight(first, " \t")
			if (strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "}")) &&
				len(strings.Fields(body)) >= minCommentedOutWords && (strings.Contains(body, "=") || strings.Contains(body, "(")) {
				flag = true
			}
		}
		if flag {
			out = append(out, mk(s.Path, c.Line, "commented-out-code", "warn",
				"commented-out code — delete it; git keeps the history"))
		}
	}
	return out
}

var numRe = regexp.MustCompile(`0[xX][0-9a-fA-F]+|\d+(?:\.\d+)?(?:[eE][+-]?\d+)?`)

var jstsConstRe = regexp.MustCompile(`(?:^|[;{])\s*(?:export\s+)?const\s+[A-Za-z_$][\w$]*\s*(?::[^=;]+)?=\s*[+-]?\s*$`)
var jstsConstEnd = regexp.MustCompile(`^\s*(?:;|$)`)
var jstsConfigPropertyRe = regexp.MustCompile(`(?:^|[{,])\s*[A-Za-z_$][\w$]*\s*:\s*[+-]?\s*$`)
var jstsConfigPropertyEnd = regexp.MustCompile(`^\s*[,}]`)
var phpDefineRe = regexp.MustCompile(`^\s*define\s*\(`)

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func allowedNum(line string, a, b int) bool {
	lit := line[a:b]
	if lit == "0" || lit == "1" || lit == "2" {
		return true
	}
	if a > 0 {
		p := line[a-1]
		if isWordByte(p) || p == '.' || p == '$' {
			return true
		}
		if p == '-' && lit == "1" {
			if a < 2 {
				return true
			}
			pp := line[a-2]
			if !isWordByte(pp) && pp != ')' && pp != ']' {
				return true
			}
		}
	}
	if b < len(line) {
		nt := line[b]
		if isWordByte(nt) {
			return true
		}
		if a > 0 && line[a-1] == '[' && nt == ']' {
			return true
		}
	}
	return false
}

func magicNumberRule(s *Source) []Finding {
	if s.IsTest {
		return nil
	}
	isConfig := s.Lang == LangJSTS && isJSConfigFile(s.ActualPath)
	if s.Lang == LangJSTS && s.AST != nil {
		return astJSMagicNumbers(s, isConfig)
	}
	var out []Finding
	inConstBlock := false
	inEnumBlock := false
	for i, line := range s.MaskedLines {
		t := strings.TrimSpace(line)
		switch s.Lang {
		case LangGo:
			if inConstBlock {
				if t == ")" {
					inConstBlock = false
				}
				continue
			}
			if strings.HasPrefix(t, "const (") {
				inConstBlock = true
				continue
			}
			if strings.HasPrefix(t, "const ") || strings.HasPrefix(t, "var ") || strings.HasPrefix(t, "import") || strings.HasPrefix(t, "package ") {
				continue
			}
		case LangJSTS:
			if inEnumBlock {
				if strings.HasPrefix(t, "}") {
					inEnumBlock = false
				}
				continue
			}
			if strings.Contains(t, "enum ") && strings.HasSuffix(t, "{") {
				inEnumBlock = true
				continue
			}
		case LangPHP:
			if phpDefineRe.MatchString(line) {
				continue
			}
		}
		for _, m := range numRe.FindAllStringIndex(line, -1) {
			if allowedNum(line, m[0], m[1]) {
				continue
			}
			if s.Lang == LangJSTS && jstsConstRe.MatchString(line[:m[0]]) && jstsConstEnd.MatchString(line[m[1]:]) {
				continue
			}
			if isConfig && isDirectConfigProperty(s.MaskedLines, i, m[0], m[1]) {
				continue
			}
			out = append(out, magicNumberFinding(s, i+1, line[m[0]:m[1]]))
		}
	}
	return out
}

func isDirectConfigProperty(lines []string, row, start, end int) bool {
	if !jstsConfigPropertyRe.MatchString(lines[row][:start]) {
		return false
	}
	if tail := strings.TrimSpace(lines[row][end:]); tail != "" {
		return jstsConfigPropertyEnd.MatchString(tail)
	}
	for _, line := range lines[row+1:] {
		if tail := strings.TrimSpace(line); tail != "" {
			return jstsConfigPropertyEnd.MatchString(tail)
		}
	}
	return false
}

func magicNumberFinding(s *Source, line int, literal string) Finding {
	return mk(s.Path, line, "magic-number", "warn",
		fmt.Sprintf("magic number %s — give this value a name that describes its purpose", literal))
}

var relImportRe = regexp.MustCompile(`\b(from|import|require|export)[\s(]*['"](\.\./[^'"]*)['"]`)

func regexRelativeImport(s *Source) []Finding {
	if s.Lang != LangJSTS {
		return nil
	}
	var out []Finding
	for i, line := range s.RawLines {
		for _, m := range relImportRe.FindAllStringSubmatchIndex(line, -1) {
			if s.MaskedLines[i][m[2]:m[3]] == line[m[2]:m[3]] {
				out = appendRelativeImport(out, s, i+1, line[m[4]:m[5]])
			}
		}
	}
	return out
}
