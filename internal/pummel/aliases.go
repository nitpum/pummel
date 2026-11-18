package pummel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type aliasConfig struct {
	Extends         string `json:"extends"`
	CompilerOptions struct {
		AllowJS *bool               `json:"allowJs"`
		BaseURL *string             `json:"baseUrl"`
		Paths   map[string][]string `json:"paths"`
	} `json:"compilerOptions"`
}

type aliases struct {
	paths   map[string][]string
	pathDir string
	baseDir string
	allowJS bool
}

func findAliases(path string) aliases {
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return aliases{}
	}
	isJS := isJavaScriptFile(path)
	configs := []string{"tsconfig.json", "jsconfig.json"}
	if isJS {
		configs = []string{"jsconfig.json", "tsconfig.json"}
	}
	for {
		for _, name := range configs {
			config := filepath.Join(dir, name)
			if _, err := os.Stat(config); err != nil {
				continue
			}
			mappings := readAliases(config, name == "jsconfig.json", map[string]bool{})
			if isJS && !mappings.allowJS {
				return aliases{}
			}
			return mappings
		}
		parent := filepath.Dir(dir)
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil || parent == dir {
			return aliases{}
		}
		dir = parent
	}
}

func isJavaScriptFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".mjs", ".cjs":
		return true
	}
	return false
}

func readAliases(path string, allowJS bool, visited map[string]bool) aliases {
	mappings := aliases{allowJS: allowJS}
	if visited[path] {
		return mappings
	}
	visited[path] = true
	data, err := os.ReadFile(path)
	if err != nil {
		return mappings
	}
	var cfg aliasConfig
	if json.Unmarshal(normalizeJSONC(data), &cfg) != nil {
		return mappings
	}
	dir := filepath.Dir(path)
	if strings.HasPrefix(cfg.Extends, "./") || strings.HasPrefix(cfg.Extends, "../") {
		base := filepath.Join(dir, filepath.FromSlash(cfg.Extends))
		if !strings.HasSuffix(base, ".json") {
			base += ".json"
		}
		mappings = readAliases(base, allowJS, visited)
	}
	opts := cfg.CompilerOptions
	if opts.AllowJS != nil {
		mappings.allowJS = *opts.AllowJS
	}
	if opts.BaseURL != nil {
		mappings.baseDir = resolvePath(dir, *opts.BaseURL)
	}
	if opts.Paths != nil {
		mappings.paths = opts.Paths
		mappings.pathDir = dir
	}
	return mappings
}

func normalizeJSONC(src []byte) []byte {
	out := make([]byte, 0, len(src))
	last := -1
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '"':
			start := i
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' {
					i++
				}
				i++
			}
			out = append(out, src[start:min(i+1, len(src))]...)
			last = len(out) - 1
			continue
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i < len(src) && src[i] != '\n' {
				i++
			}
			c = '\n'
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i++
			c = ' '
		case (c == '}' || c == ']') && last >= 0 && out[last] == ',':
			out[last] = ' '
		}
		out = append(out, c)
		if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
			last = len(out) - 1
		}
	}
	return out
}

func resolvePath(dir, path string) string {
	path = filepath.FromSlash(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

func (a aliases) match(target string) string {
	dir := a.baseDir
	if dir == "" {
		dir = a.pathDir
	}
	var matches []string
	for alias, paths := range a.paths {
		if alias == "" || strings.HasPrefix(alias, "./") || strings.HasPrefix(alias, "../") {
			continue
		}
		if !strings.Contains(alias, "*") {
			if a.resolve(alias, dir) == target {
				matches = append(matches, alias)
			}
			continue
		}
		for _, path := range paths {
			prefix, suffix, wildcard := strings.Cut(path, "*")
			if !wildcard || strings.Count(alias, "*") != 1 || strings.Contains(suffix, "*") {
				continue
			}
			pattern := filepath.ToSlash(resolvePath(dir, prefix+"*"+suffix))
			start, end, _ := strings.Cut(pattern, "*")
			for _, form := range moduleForms(target) {
				middle, matched := strings.CutPrefix(filepath.ToSlash(form), start)
				if !matched {
					continue
				}
				value, matched := strings.CutSuffix(middle, end)
				if matched && value != "" {
					specifier := strings.Replace(alias, "*", value, 1)
					if a.resolve(specifier, dir) == target {
						matches = append(matches, specifier)
					}
				}
			}
		}
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	return matches[0]
}

func (a aliases) resolve(specifier, dir string) string {
	paths, exact := a.paths[specifier]
	wildcard := ""
	if !exact {
		bestPrefix := -1
		bestPattern := ""
		for pattern, candidates := range a.paths {
			prefix, suffix, ok := strings.Cut(pattern, "*")
			if !ok || len(prefix) < bestPrefix || (len(prefix) == bestPrefix && pattern > bestPattern) || strings.Contains(suffix, "*") {
				continue
			}
			suffixPart, ok := strings.CutPrefix(specifier, prefix)
			if !ok {
				continue
			}
			if match, ok := strings.CutSuffix(suffixPart, suffix); ok {
				bestPrefix, bestPattern, paths, wildcard = len(prefix), pattern, candidates, match
			}
		}
	}
	for _, path := range paths {
		if target := resolveModule(resolvePath(dir, strings.Replace(path, "*", wildcard, 1))); target != "" {
			return target
		}
	}
	return ""
}

var sourceExtensions = []string{".ts", ".tsx", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}

func moduleForms(path string) []string {
	ext := filepath.Ext(path)
	if DetectLang(path) != LangJSTS {
		return []string{path}
	}
	stem := strings.TrimSuffix(path, ext)
	forms := []string{stem, path}
	if filepath.Base(stem) == "index" {
		forms = append(forms, filepath.Dir(stem))
	}
	return forms
}

func resolveModule(path string) string {
	candidates := []string{path}
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	switch filepath.Ext(path) {
	case ".js":
		candidates = append(candidates, stem+".ts", stem+".tsx")
	case ".mjs":
		candidates = append(candidates, stem+".mts")
	case ".cjs":
		candidates = append(candidates, stem+".cts")
	}
	for _, ext := range sourceExtensions {
		candidates = append(candidates, path+ext)
	}
	for _, ext := range sourceExtensions {
		candidates = append(candidates, filepath.Join(path, "index"+ext))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}

func appendRelativeImport(out []Finding, s *Source, line int, path string) []Finding {
	if !strings.HasPrefix(path, "../") {
		return out
	}
	mappings := s.importAliases()
	if len(mappings.paths) == 0 {
		return out
	}
	dir, err := filepath.Abs(filepath.Dir(s.ActualPath))
	if err != nil {
		return out
	}
	target := resolveModule(resolvePath(dir, path))
	if target != "" {
		if alias := mappings.match(target); alias != "" {
			out = append(out, mk(s.Path, line, "relative-import", "warn",
				fmt.Sprintf("relative import %q can use configured alias %q", path, alias)))
		}
	}
	return out
}
