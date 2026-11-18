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

func isJSConfigFile(path string) bool {
	path = strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	dir, base := filepath.Split(path)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	return stem == "config" || strings.HasSuffix(stem, ".config") ||
		strings.Contains("/"+dir, "/config/")
}

func IsTestFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch DetectLang(path) {
	case LangGo:
		return strings.HasSuffix(base, "_test.go")
	case LangJSTS:
		if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
			return true
		}
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		for _, suffix := range []string{".fixture", ".mock", ".seed"} {
			if strings.HasSuffix(stem, suffix) {
				return true
			}
		}
		dir := filepath.Dir(path)
		for {
			switch strings.ToLower(filepath.Base(dir)) {
			case "test", "tests", "__tests__", "testdata", "fixtures", "__fixtures__", "mocks", "__mocks__", "seeds", "__seeds__":
				return true
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return false
}
