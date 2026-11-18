//go:build cgo

package pummel

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjs "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tsphp "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tsts "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type AST struct {
	Parser *sitter.Parser
	Tree   *sitter.Tree
	Src    []byte
}

type gkind int

const (
	gNone gkind = iota
	gJS
	gTS
	gTSX
	gGo
	gPHP
)

func grammarKind(path string, lang Lang) gkind {
	if lang == LangGo {
		return gGo
	}
	if lang == LangPHP {
		return gPHP
	}
	if lang == LangJSTS {
		base := strings.ToLower(filepath.Base(path))
		switch {
		case strings.HasSuffix(base, ".tsx"):
			return gTSX
		case strings.HasSuffix(base, ".ts"), strings.HasSuffix(base, ".mts"), strings.HasSuffix(base, ".cts"):
			return gTS
		default:
			return gJS
		}
	}
	return gNone
}

func ParseAST(path string, lang Lang, src []byte) *AST {
	kind := grammarKind(path, lang)
	gl := langForKind(kind)
	if gl == nil {
		return nil
	}
	p := sitter.NewParser()
	if err := p.SetLanguage(gl); err != nil {
		p.Close()
		return nil
	}
	tree := p.Parse(src, nil)
	if tree == nil {
		p.Close()
		return nil
	}
	return &AST{Parser: p, Tree: tree, Src: src}
}

func (a *AST) Close() {
	if a == nil {
		return
	}
	a.Tree.Close()
	a.Parser.Close()
}

var (
	queryMu    sync.Mutex
	queryCache = map[gkind]map[string]*sitter.Query{}
)

const (
	jsFnQuerySrc = `
(function_declaration name: (identifier) @fn)
(generator_function_declaration name: (identifier) @fn)
(method_definition name: (property_identifier) @fn)
(variable_declarator name: (identifier) @fn value: [(arrow_function) (function_expression)])
(pair key: (property_identifier) @fn value: [(arrow_function) (function_expression)])
(assignment_expression left: (identifier) @fn right: [(arrow_function) (function_expression)])
`
	jsImportQuerySrc = `
(import_statement source: (string (string_fragment) @path))
(export_statement source: (string (string_fragment) @path))
(call_expression function: (identifier) @fn arguments: (arguments . (string (string_fragment) @path)))
`
	goFnQuerySrc = `
(function_declaration name: (identifier) @fn)
(method_declaration name: (field_identifier) @fn)
`
	phpFnQuerySrc = `
(function_definition name: (name) @fn)
(method_declaration name: (name) @fn)
`
)

func getQuery(kind gkind, src string) *sitter.Query {
	queryMu.Lock()
	defer queryMu.Unlock()
	m := queryCache[kind]
	if m == nil {
		m = map[string]*sitter.Query{}
		queryCache[kind] = m
	}
	if q, ok := m[src]; ok {
		return q
	}
	q, err := sitter.NewQuery(langForKind(kind), src)
	if err != nil {
		return nil
	}
	m[src] = q
	return q
}

func langForKind(kind gkind) *sitter.Language {
	switch kind {
	case gGo:
		return sitter.NewLanguage(tsgo.Language())
	case gPHP:
		return sitter.NewLanguage(tsphp.LanguagePHP())
	case gJS:
		return sitter.NewLanguage(tsjs.Language())
	case gTS:
		return sitter.NewLanguage(tsts.LanguageTypescript())
	case gTSX:
		return sitter.NewLanguage(tsts.LanguageTSX())
	}
	return nil
}

type nodeText struct {
	node *sitter.Node
	text string
	line int
}

func runQuery(kind gkind, querySrc string, ast *AST) [][]nodeText {
	q := getQuery(kind, querySrc)
	if q == nil {
		return nil
	}
	qc := sitter.NewQueryCursor()
	defer qc.Close()
	var out [][]nodeText
	it := qc.Matches(q, ast.Tree.RootNode(), ast.Src)
	for {
		m := it.Next()
		if m == nil {
			break
		}
		var caps []nodeText
		for _, c := range m.Captures {
			caps = append(caps, nodeText{
				node: &c.Node,
				text: string(ast.Src[c.Node.StartByte():c.Node.EndByte()]),
				line: int(c.Node.StartPosition().Row) + 1,
			})
		}
		out = append(out, caps)
	}
	return out
}

func astVerbFirst(s *Source) []Finding {
	if s.AST == nil {
		return nil
	}
	kind := grammarKind(s.Path, s.Lang)
	var qsrc string
	switch kind {
	case gJS, gTS, gTSX:
		qsrc = jsFnQuerySrc
	case gGo:
		qsrc = goFnQuerySrc
	case gPHP:
		qsrc = phpFnQuerySrc
	default:
		return nil
	}
	var out []Finding
	for _, caps := range runQuery(kind, qsrc, s.AST) {
		for _, c := range caps {
			name := strings.Trim(c.text, "\"'`")
			if name == "" || skipFuncName(s.Lang, name) {
				continue
			}
			if nounWords[firstToken(name)] {
				out = append(out, mk(s.Path, c.line, "verb-first", "warn",
					fmt.Sprintf("function %q should start with a verb (e.g. fetch%s / load%s)", name, upFirst(name), upFirst(name))))
			}
		}
	}
	return out
}

func astRelativeImport(s *Source) []Finding {
	if s.AST == nil || s.Lang != LangJSTS {
		return nil
	}
	kind := grammarKind(s.Path, s.Lang)
	if kind != gJS && kind != gTS && kind != gTSX {
		return nil
	}
	var out []Finding
	for _, caps := range runQuery(kind, jsImportQuerySrc, s.AST) {
		var fn, path nodeText
		haveFn, havePath := false, false
		for _, c := range caps {
			switch c.node.Kind() {
			case "string_fragment":
				path, havePath = c, true
			case "identifier":
				fn, haveFn = c, true
			}
		}
		if haveFn && fn.text != "require" && fn.text != "import" {
			continue
		}
		if !havePath {
			continue
		}
		if strings.HasPrefix(path.text, "../") {
			out = append(out, mk(s.Path, path.line, "relative-import", "warn",
				fmt.Sprintf("relative import %q — prefer an absolute path (e.g. @/…)", path.text)))
		}
	}
	return out
}
