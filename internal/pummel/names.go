package pummel

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	jstsFuncRe    = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s*\*?\s*([A-Za-z_$][\w$]*)`)
	jstsConstFnRe = regexp.MustCompile(`^\s*(?:export\s+)?const\s+([A-Za-z_$][\w$]*)\s*(?::[^=]+)?=\s*(?:async\s*)?(?:function\b|\()`)
	jstsMethodRe  = regexp.MustCompile(`^\s+(?:(?:public|private|protected|readonly|static|async|override|abstract)\s+)*(?:get\s+|set\s+|\*)?\s*([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*\{\s*$`)
	goFuncRe      = regexp.MustCompile(`^func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)\(`)
	phpFuncRe     = regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|abstract|final)\s+)*function\s+([A-Za-z_]\w*)\s*\(`)
)

var methodKeywordSkip = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"function": true, "return": true, "else": true, "try": true, "do": true,
	"new": true, "delete": true, "typeof": true, "void": true, "in": true,
	"of": true, "export": true, "import": true, "constructor": true,
}

var reactAllow = map[string]bool{
	"componentDidMount": true, "componentWillUnmount": true,
	"componentDidCatch": true, "componentWillReceiveProps": true,
	"componentWillUpdate": true, "componentWillMount": true,
}

var goAllow = map[string]bool{"main": true, "init": true, "String": true, "Error": true}

const verbList = `
activate add append apply assert assign attach authenticate begin bind build calculate call can
cancel capitalize capture change check choose clamp classify clear clone close collapse collect
combine compare compile compose compress compute concat connect convert copy count create
deactivate debounce decode decompress decrement decrypt define delay delete derive detach detect
disable dispatch display divide does edit emit enable encode encrypt ensure escape exec execute
expand export extract fetch filter find flatten flush force format freeze generate get group
guess handle has hash hide identify increment infer init initialize inject insert invoke is join
list listen load lock log lowercase make map mask match memoize merge minify mount move multiply
navigate normalize notify observe omit open pad pair parse patch pause pick pop preload prepend
preprocess produce push query randomize read redirect reduce refresh register remove render
replace reset resize resolve restrict retrieve retry round run sanitize save scaffold schedule
scroll search select separate serialize set setup share shift should show shuffle skip sleep
slugify sort split start stop store stringify strip subscribe subtract sum swap sync test
throttle to toggle track transform translate trigger trim truncate unbind unescape unlock
unmount unregister unsubscribe unwrap update uppercase upsert use validate verify wait warn
watch will wrap write
`

var verbWords = map[string]bool{}
var nounWords = map[string]bool{}

var nounList = []string{
	"user", "users", "data", "config", "info", "result", "results", "helper",
	"helpers", "util", "utils", "utility", "string", "str", "number", "num",
	"array", "arr", "list", "map", "dict", "item", "items", "value",
	"values", "key", "keys", "entry", "entries", "temp", "tmp", "buf",
	"buffer", "byte", "bytes", "char", "bool", "boolean", "int", "float",
	"double", "response", "resp", "request", "req", "header", "headers",
	"token", "tokens", "cache", "file", "files", "path", "paths", "dir",
	"directory", "folder", "name", "names", "id", "ids", "index", "count",
	"counter", "total", "sum", "amount", "price", "cost", "date", "day",
	"month", "year", "time", "timestamp", "message", "messages", "msg",
	"error", "errors", "err", "event", "events", "state", "props",
	"context", "ctx", "menu", "button", "btn", "dialog", "modal", "page",
	"pages", "view", "views", "table", "row", "rows", "column", "col",
	"cell", "form", "field", "fields", "input", "output", "label", "text",
	"title", "subtitle", "description", "image", "img", "picture", "video",
	"audio", "icon", "icons", "style", "theme", "color", "colors", "layout",
	"container", "wrapper", "content", "body", "footer", "sidebar", "nav",
	"navigation", "link", "links", "url", "uri", "endpoint", "api",
	"service", "services", "client", "server", "db", "database", "query",
	"schema", "model", "models", "entity", "entities", "record", "records",
	"repo", "repository", "controller", "middleware", "router", "route",
	"routes", "provider", "factory", "builder", "adapter", "callback",
	"cb", "promise", "session", "cookie", "cookies", "auth", "permission",
	"role", "roles", "account", "profile", "cart", "order", "orders",
	"product", "products", "payment", "invoice", "report", "reports",
	"log", "logs", "logger", "flag", "flags", "option", "options", "param",
	"params", "argument", "args", "settings", "preference", "preferences",
	"snapshot", "backup", "manager", "handlers", "handler",
}

func init() {
	for _, word := range strings.Fields(verbList) {
		verbWords[word] = true
	}
	for _, w := range nounList {
		nounWords[w] = true
	}
}

func extractFuncName(lang Lang, line string) string {
	switch lang {
	case LangGo:
		if m := goFuncRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	case LangJSTS:
		if m := jstsFuncRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
		if m := jstsConstFnRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
		if m := jstsMethodRe.FindStringSubmatch(line); m != nil && !methodKeywordSkip[m[1]] {
			return m[1]
		}
	case LangPHP:
		if m := phpFuncRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

func skipFuncName(lang Lang, name string) bool {
	switch lang {
	case LangGo:
		if goAllow[name] {
			return true
		}
		for _, p := range []string{"Test", "Benchmark", "Example", "Fuzz"} {
			if strings.HasPrefix(name, p) {
				return true
			}
		}
	case LangJSTS:
		if reactAllow[name] || name == "main" {
			return true
		}
	case LangPHP:
		if strings.HasPrefix(name, "__") {
			return true
		}
	}
	return false
}

func splitName(name string) []string {
	var parts []string
	var cur strings.Builder
	prevLower := false
	flush := func() {
		if cur.Len() > 0 {
			parts = append(parts, cur.String())
			cur.Reset()
		}
	}
	for _, r := range name {
		switch {
		case r == '_' || r == '$':
			flush()
			prevLower = false
		case unicode.IsUpper(r) && prevLower:
			flush()
			cur.WriteRune(r)
			prevLower = false
		default:
			cur.WriteRune(r)
			prevLower = unicode.IsLower(r) || unicode.IsDigit(r)
		}
	}
	flush()
	return parts
}

func firstToken(name string) string {
	parts := splitName(name)
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(parts[0])
}

func hasNounPrefix(name string) bool {
	word := firstToken(name)
	return nounWords[word] && !verbWords[word]
}

func regexVerbFirst(s *Source) []Finding {
	var out []Finding
	for i, line := range s.MaskedLines {
		name := extractFuncName(s.Lang, line)
		if name == "" || skipFuncName(s.Lang, name) {
			continue
		}
		if hasNounPrefix(name) {
			out = append(out, mk(s.Path, i+1, "verb-first", "warn",
				fmt.Sprintf("function %q should start with a verb (e.g. fetch%s / load%s)", name, upFirst(name), upFirst(name))))
		}
	}
	return out
}

func upFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
