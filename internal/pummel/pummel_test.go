package pummel

import (
	"strings"
	"testing"
)

func findingsByRule(fs []Finding, rule string) []Finding {
	var out []Finding
	for _, f := range fs {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}

func runRule(t *testing.T, rule, path, src string) []Finding {
	t.Helper()
	s := Load(path, []byte(src), false)
	var got []Finding
	for _, r := range AllRules {
		if r.ID == rule {
			got = r.Fn(s)
		}
	}
	return got
}

func TestScanMasksStringsAndComments(t *testing.T) {
	src := "const u = \"http://x\"; // real comment\n" + "var s = `multi\nline`;\n/* block\ncomment */\n"
	res := scan(src, LangJSTS)
	if got := len(res.Comments); got != 2 {
		t.Fatalf("comments = %d, want 2 (%+v)", got, res.Comments)
	}
	if res.Comments[0].Text != "real comment" || res.Comments[0].Line != 1 {
		t.Fatalf("comment0 = %+v", res.Comments[0])
	}
	if strings.Contains(res.Masked, "http") {
		t.Fatalf("string not masked: %q", res.Masked)
	}
	if strings.Contains(res.Masked, "real comment") {
		t.Fatalf("comment not masked: %q", res.Masked)
	}
}

func TestCommentPrefix(t *testing.T) {
	fs := runRule(t, "comment-prefix", "a.ts", "// TODO: x\n// plain text\n/* NOTE: y */\n// ----\n")
	if got := len(findingsByRule(fs, "comment-prefix")); got != 1 {
		t.Fatalf("findings = %d, want 1 (%+v)", got, fs)
	}
}

func TestTicketRef(t *testing.T) {
	fs := runRule(t, "ticket-ref", "a.ts", "// NOTE: PROJ-123 fixed\n// TODO: PROJ-123 later\n// NOTE: UTF-8 ok\n// NOTE: see MR 42\n")
	if got := len(findingsByRule(fs, "ticket-ref")); got != 2 {
		t.Fatalf("findings = %d, want 2 (%+v)", got, fs)
	}
}

func TestCommentedOutCode(t *testing.T) {
	fs := runRule(t, "commented-out-code", "a.go", "// fmt.Println(\"hi\")\n// return the items when done\n// const x = 5\n")
	if got := len(findingsByRule(fs, "commented-out-code")); got != 2 {
		t.Fatalf("findings = %d, want 2 (%+v)", got, fs)
	}
}

func TestVerbFirst(t *testing.T) {
	fs := runRule(t, "verb-first", "a.ts", "function userInfo() {}\nfunction fetchUserData() {}\nconst dataLoader = () => {}\nclass A { componentDidMount() {} render() {} }\n")
	if got := len(findingsByRule(fs, "verb-first")); got != 2 {
		t.Fatalf("findings = %d, want 2 (%+v)", got, fs)
	}
	fs = runRule(t, "verb-first", "a.go", "func userInfo() {}\nfunc (s *S) fetchUserData() {}\nfunc TestUserInfo(t *testing.T) {}\nfunc main() {}\nfunc String() string { return \"\" }\n")
	if got := len(findingsByRule(fs, "verb-first")); got != 1 {
		t.Fatalf("go findings = %d, want 1 (%+v)", got, fs)
	}
	fs = runRule(t, "verb-first", "a.php", "<?php\nfunction userInfo() {}\nfunction fetchUserData() {}\nfunction __construct() {}\n")
	if got := len(findingsByRule(fs, "verb-first")); got != 1 {
		t.Fatalf("php findings = %d, want 1 (%+v)", got, fs)
	}
}

func TestMagicNumber(t *testing.T) {
	fs := runRule(t, "magic-number", "a.ts", "const DASHBOARD_LOAD_DELAY_MS = 5000;\nconst delay = 3000;\nlet x = 5;\nlet ok = 1;\nlet m = -1;\n")
	if got := len(findingsByRule(fs, "magic-number")); got != 1 {
		t.Fatalf("findings = %d, want 1 (%+v)", got, fs)
	}
	fs = runRule(t, "magic-number", "a.go", "const (\n\tA = 5\n\tB = 10\n)\nvar buf [4096]byte\nn := 42\n")
	if got := len(findingsByRule(fs, "magic-number")); got != 1 {
		t.Fatalf("go findings = %d, want 1 (%+v)", got, fs)
	}
	s := Load("a_test.go", []byte("n := 42\n"), true)
	if got := len(magicNumberRule(s)); got != 0 {
		t.Fatalf("test file findings = %d, want 0", got)
	}
}

func TestRelativeImport(t *testing.T) {
	fs := runRule(t, "relative-import", "a.ts", "import x from '../../utils/date';\nimport y from '@/utils/date';\nconst s = \"../notimport\";\nrequire('../lib/x');\n")
	if got := len(findingsByRule(fs, "relative-import")); got != 2 {
		t.Fatalf("findings = %d, want 2 (%+v)", got, fs)
	}
	fs = runRule(t, "relative-import", "a.php", "<?php use App\\User;\n")
	if got := len(findingsByRule(fs, "relative-import")); got != 0 {
		t.Fatalf("php findings = %d, want 0", got)
	}
}

func TestASTMultilineImport(t *testing.T) {
	src := "import {\n  a,\n  b,\n} from '../lib/x';\n"
	s := Load("a.ts", []byte(src), false)
	defer s.Close()
	if s.AST == nil {
		t.Skip("AST unavailable (non-cgo build)")
	}
	fs := astRelativeImport(s)
	if got := len(findingsByRule(fs, "relative-import")); got != 1 {
		t.Fatalf("ast findings = %d, want 1 (%+v)", got, fs)
	}
}

func TestASTVerbFirstExtraShapes(t *testing.T) {
	src := "let dataHelper = () => {};\nconst obj = { configLoader: function () {} };\n"
	s := Load("a.ts", []byte(src), false)
	defer s.Close()
	if s.AST == nil {
		t.Skip("AST unavailable (non-cgo build)")
	}
	fs := astVerbFirst(s)
	if got := len(findingsByRule(fs, "verb-first")); got != 2 {
		t.Fatalf("ast findings = %d, want 2 (%+v)", got, fs)
	}
}

func TestCommentLength(t *testing.T) {
	long := "// " + strings.Repeat("a", 130)
	fs := runRule(t, "comment-length", "a.ts", long+"\n// short\n")
	if got := len(findingsByRule(fs, "comment-length")); got != 1 {
		t.Fatalf("findings = %d, want 1", got)
	}
}

func TestEncodeFindingsTOON(t *testing.T) {
	if got := EncodeFindings(nil); got != "[]" {
		t.Fatalf("empty = %q, want []", got)
	}
	fs := []Finding{
		{File: "src/app.ts", Line: 12, Rule: "comment-prefix", Severity: "warn", Message: "comment must start with TODO:, NOTE:, or FIXME:"},
		{File: "weird.php", Line: 3, Rule: "ticket-ref", Severity: "warn", Message: "has \"quotes\", commas,\nand a tab\t"},
	}
	got := EncodeFindings(fs)
	want := "[2]{file,line,rule,severity,message}:\n" +
		"src/app.ts,12,comment-prefix,warn,\"comment must start with TODO:, NOTE:, or FIXME:\"\n" +
		"weird.php,3,ticket-ref,warn,\"has \\\"quotes\\\", commas,\\nand a tab\\t\""
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	for _, s := range []string{"123", "true", "-x", "#c", " padded", "a:b"} {
		if !toonNeedsQuote(s) {
			t.Errorf("toonNeedsQuote(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"warn", "src/app.ts", "hello world"} {
		if toonNeedsQuote(s) {
			t.Errorf("toonNeedsQuote(%q) = true, want false", s)
		}
	}
}
