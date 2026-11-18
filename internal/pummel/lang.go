package pummel

import (
	"path/filepath"
	"strings"
)

type Lang int

const (
	LangNone Lang = iota
	LangJSTS
	LangGo
	LangPHP
)

func (l Lang) String() string {
	switch l {
	case LangJSTS:
		return "js/ts"
	case LangGo:
		return "go"
	case LangPHP:
		return "php"
	}
	return "none"
}

func DetectLang(path string) Lang {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx", ".mts", ".cts":
		return LangJSTS
	case ".go":
		return LangGo
	case ".php", ".phtml":
		return LangPHP
	}
	return LangNone
}

func IsTestFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch DetectLang(path) {
	case LangGo:
		return strings.HasSuffix(base, "_test.go")
	case LangJSTS:
		return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
	}
	return false
}
