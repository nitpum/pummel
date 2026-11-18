package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pummel/internal/pummel"
)

const maxFileSize = 1 << 20
const binarySniffLen = 8192

var ignoreDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "out": true, "coverage": true, ".next": true,
	".turbo": true, ".cache": true, ".nyc_output": true, "tmp": true,
	"testdata": true,
}

type target struct {
	display string
	actual  string
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("pummel", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "JSON output (for agents)")
	toonOut := fs.Bool("toon", false, "TOON output (token-efficient, toonformat.dev)")
	diffMode := fs.Bool("diff", false, "review only lines changed vs --base (git)")
	base := fs.String("base", "HEAD", "git ref to diff against (with --diff)")
	disable := fs.String("disable", "", "comma-separated rule IDs to skip")
	listRules := fs.Bool("list-rules", false, "list available rules")
	fs.Usage = usage
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *listRules {
		for _, r := range pummel.AllRules {
			fmt.Printf("%-18s %-12s %s\n", r.ID, "["+r.Langs+"]", r.Description)
		}
		return 0
	}

	disabled := map[string]bool{}
	for _, id := range strings.Split(*disable, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			disabled[id] = true
		}
	}

	var targets []target
	addedLines := map[string]map[int]bool{}

	if *diffMode {
		dir := "."
		if fs.NArg() > 0 {
			dir = fs.Arg(0)
		}
		diff, err := pummel.GitDiffAdded(dir, *base)
		if err != nil {
			fmt.Fprintf(os.Stderr, "pummel: git diff failed: %v\n", err)
			return 2
		}
		abs, _ := filepath.Abs(dir)
		for p, lines := range diff {
			if pummel.DetectLang(p) == pummel.LangNone {
				continue
			}
			targets = append(targets, target{display: p, actual: filepath.Join(abs, p)})
			addedLines[p] = lines
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].display < targets[j].display })
	} else {
		paths := fs.Args()
		if len(paths) == 0 {
			paths = []string{"."}
		}
		for _, p := range paths {
			ts, err := collect(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "pummel: %v\n", err)
				return 2
			}
			targets = append(targets, ts...)
		}
	}

	findings := []pummel.Finding{}
	for _, t := range targets {
		content, err := os.ReadFile(t.actual)
		if err != nil {
			continue
		}
		if isBinary(content) {
			continue
		}
		src := pummel.LoadFile(t.display, t.actual, content, pummel.IsTestFile(t.actual))
		fs := pummel.Run(src, disabled)
		src.Close()
		lines, limit := addedLines[t.display]
		for _, f := range fs {
			if limit && !lines[f.Line] {
				continue
			}
			findings = append(findings, f)
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})

	if *toonOut {
		fmt.Println(pummel.EncodeFindings(findings))
	} else if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(findings); err != nil {
			fmt.Fprintf(os.Stderr, "pummel: %v\n", err)
			return 2
		}
	} else {
		warn, info := 0, 0
		for _, f := range findings {
			fmt.Printf("%s:%d [%s/%s] %s\n", f.File, f.Line, f.Rule, f.Severity, f.Message)
			if f.Severity == "warn" {
				warn++
			} else {
				info++
			}
		}
		if len(findings) == 0 {
			fmt.Println("clean — no findings")
		} else {
			fmt.Printf("\n%d findings (%d warn, %d info)\n", len(findings), warn, info)
		}
	}

	if len(findings) > 0 {
		return 1
	}
	return 0
}

func usage() {
	fmt.Fprintf(os.Stderr, `pummel — deterministic code review before the agent sees it

usage:
  pummel [flags] [path ...]     review files/directories (default: .)
  pummel --diff [dir]           review only changed lines vs --base (default HEAD)
  pummel --diff main            diff against main instead of HEAD

flags:
  --json          machine-readable output for agents
  --toon          token-efficient output for agents (toonformat.dev)
  --base REF      git ref for --diff (default HEAD)
  --disable IDS   skip rules, e.g. --disable magic-number,verb-first
  --list-rules    list rule IDs
  --help          this help

exit codes: 0 clean, 1 findings, 2 error
`)
}

func collect(root string) ([]target, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if pummel.DetectLang(root) != pummel.LangNone {
			return []target{{display: root, actual: root}}, nil
		}
		return nil, nil
	}
	var out []target
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ignoreDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if strings.Contains(name, ".min.") {
			return nil
		}
		if pummel.DetectLang(name) == pummel.LangNone {
			return nil
		}
		if fi, err := d.Info(); err == nil && fi.Size() > maxFileSize {
			return nil
		}
		out = append(out, target{display: path, actual: path})
		return nil
	})
	return out, err
}

func isBinary(b []byte) bool {
	n := len(b)
	if n > binarySniffLen {
		n = binarySniffLen
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return true
		}
	}
	return false
}
