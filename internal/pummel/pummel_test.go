package pummel

import (
	"os"
	"path/filepath"
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
	defer s.Close()
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
	fs := runRule(t, "verb-first", "a.ts", "function userInfo() {}\nfunction fetchUserData() {}\nconst dataLoader = () => {}\nconst setSales = () => {}\nclass A { componentDidMount() {} render() {} }\n")
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

func TestVerbFirstAcceptsWordsThatCanAlsoBeNouns(t *testing.T) {
	src := "function listUsers() {}\nfunction mapSales() {}\nfunction countItems() {}\n" +
		"function queryOrders() {}\nfunction logPayment() {}\nfunction sumTotals() {}\n" +
		"function totalAmount() {}\nfunction frobnicate() {}\nfunction setTenders() {}\n"
	findings := runRule(t, "verb-first", "a.ts", src)
	if len(findings) != 1 || findings[0].Line != 7 {
		t.Fatalf("findings = %+v, want only totalAmount", findings)
	}
}

func TestMagicNumberExemptsDirectNamedConfigProperties(t *testing.T) {
	src := `const defaults = {
  testTimeout: 60000,
  sftp: { defaultPort: 2222 },
  timeout: wait(5000),
  precision: 4 * 5,
  delay: 4000
    + 6000,
  lastValue: 7000
};
`
	for _, path := range []string{"tool.config.cjs", "src/config/service.ts", "config.ts", "src/service.ts", "src/configuration/service.ts"} {
		want := []int{4, 5, 5, 6, 7}
		if path == "src/service.ts" || path == "src/configuration/service.ts" {
			want = []int{2, 3, 4, 5, 5, 6, 7, 8}
		}
		findings := runRule(t, "magic-number", path, src)
		if len(findings) != len(want) {
			t.Errorf("%s findings = %+v, want lines %v", path, findings, want)
			continue
		}
		for i, f := range findings {
			if f.Line != want[i] {
				t.Errorf("%s finding = %+v, want line %d", path, f, want[i])
			}
		}
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
	defer s.Close()
	if got := len(magicNumberRule(s)); got != 0 {
		t.Fatalf("test file findings = %d, want 0", got)
	}
}

func TestRelativeImport(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "tsconfig.json", `{
		// NOTE: The runtime and compiler share this alias.
		"extends": "./base",
		"compilerOptions": { "strict": true, },
	}`)
	writeTestFile(t, dir, "base.json", `{"compilerOptions":{"baseUrl":".","paths":{"@/*":["missing/*", "src/*"]}}}`)
	writeTestFile(t, dir, "src/utils/date.ts", "")
	writeTestFile(t, dir, "src/tools/load.ts", "")
	writeTestFile(t, dir, "other/tool.js", "")
	src := "import x from '../utils/date';\nimport y from '@/utils/date';\n" +
		"const s = \"continued\\\nrequire('../utils/date')\";\n" +
		"// import '../utils/date';\nrequire('../../other/tool');\n"
	s := LoadFile("src/tools/load.ts", filepath.Join(dir, "src/tools/load.ts"), []byte(src), false)
	defer s.Close()
	findings := findingsByRule(Run(s, nil), "relative-import")
	if len(findings) != 1 || findings[0].File != "src/tools/load.ts" || findings[0].Line != 1 ||
		!strings.Contains(findings[0].Message, `"@/utils/date"`) {
		t.Fatalf("findings = %+v, want configured alias on line 1", findings)
	}
	for _, path := range []string{filepath.Join(dir, "src/tools/plugin.js"), filepath.Join(t.TempDir(), "src/tools/load.ts"), "a.php"} {
		if got := runRule(t, "relative-import", path, src); len(got) != 0 {
			t.Errorf("%s findings = %+v, want none", path, got)
		}
	}
}

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestASTMultilineImport(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "tsconfig.json", `{"compilerOptions":{"paths":{"@lib/*":["lib/*"]}}}`)
	writeTestFile(t, dir, "lib/x.ts", "")
	src := "import {\n  a,\n  b,\n} from '../lib/x';\nconst lazy = import('../lib/x');\n"
	s := Load(filepath.Join(dir, "src/a.ts"), []byte(src), false)
	defer s.Close()
	if s.AST == nil {
		t.Skip("AST unavailable (non-cgo build)")
	}
	fs := astRelativeImport(s)
	if len(fs) != 2 || fs[0].Line != 4 || fs[1].Line != 5 {
		t.Fatalf("ast findings = %+v, want lines 4 and 5", fs)
	}
}

func TestJSRegexAndNumericInitializers(t *testing.T) {
	src := `const cleaned = value.replace(/\//g, '');
const valid = /^\d{4}-\d{2}-\d{2}$/.test(value);
const ratio = total / 4 / 2; // actual comment
const DIRECT = 5000; const rounded = value.toDecimalPlaces(4, 4);
const result = wait(5000);
const adjusted = BASE + 9;
const nested = { retries: 7 };
return /[/*9]/.test(value);
`
	s := Load("a.ts", []byte(src), false)
	defer s.Close()
	findings := Run(s, nil)
	comments := findingsByRule(findings, "comment-prefix")
	if len(comments) != 1 || comments[0].Line != 3 {
		t.Fatalf("comment findings = %+v, want line 3", comments)
	}
	numbers := findingsByRule(findings, "magic-number")
	want := []int{3, 4, 4, 5, 6, 7}
	if len(numbers) != len(want) {
		t.Fatalf("number findings = %+v, want lines %v", numbers, want)
	}
	for i, f := range numbers {
		if f.Line != want[i] || strings.Contains(f.Message, "TIMEOUT_MS") {
			t.Errorf("finding = %+v, want line %d and a neutral suggestion", f, want[i])
		}
	}
}

func TestTestSupportPaths(t *testing.T) {
	for _, path := range []string{"src/a.spec.ts", "src/a.fixture.ts", "src/a.mock.ts", "src/a.seed.ts",
		"src/mocks/a.ts", "src/fixtures/a.js", "src/seeds/a.ts", "src/tests/support/a.ts", "a_test.go"} {
		s := Load(path, []byte("// plain comment\nlet n = 44;\n"), IsTestFile(path))
		findings := Run(s, nil)
		s.Close()
		if len(findingsByRule(findings, "magic-number")) != 0 || len(findingsByRule(findings, "comment-prefix")) != 1 {
			t.Errorf("%s findings = %+v, want only a comment finding", path, findings)
		}
	}
	for _, path := range []string{"src/seedlings/a.ts", "src/mock-service.ts", "src/fixtures/run.go"} {
		if IsTestFile(path) {
			t.Errorf("%s classified as test support", path)
		}
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
