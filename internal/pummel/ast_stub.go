//go:build !cgo

package pummel

type AST struct{}

func ParseAST(path string, lang Lang, src []byte) *AST { return nil }

func (a *AST) Close() {}

func astVerbFirst(s *Source) []Finding { return nil }

func astRelativeImport(s *Source) []Finding { return nil }

func astJSComments(s *Source) []Comment { return nil }

func astJSMagicNumbers(s *Source) []Finding { return nil }
