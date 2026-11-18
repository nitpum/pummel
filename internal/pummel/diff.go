package pummel

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var (
	hunkRe = regexp.MustCompile(`^@@.*\+(\d+)(?:,(\d+))?`)
	fileRe = regexp.MustCompile(`^\+\+\+ b/(.+)$`)
)

func GitDiffAdded(dir, base string) (map[string]map[int]bool, error) {
	cmd := exec.Command("git", "-C", dir, "diff", "--no-color", "--unified=0", base, "--")
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, err
		}
		return nil, err
	}
	files := map[string]map[int]bool{}
	cur := ""
	for _, l := range strings.Split(string(out), "\n") {
		if m := fileRe.FindStringSubmatch(l); m != nil {
			cur = strings.TrimSpace(m[1])
			if cur == "/dev/null" {
				cur = ""
			}
			continue
		}
		if cur == "" {
			continue
		}
		if m := hunkRe.FindStringSubmatch(l); m != nil {
			start, _ := strconv.Atoi(m[1])
			cnt := 1
			if m[2] != "" {
				cnt, _ = strconv.Atoi(m[2])
			}
			if files[cur] == nil {
				files[cur] = map[int]bool{}
			}
			for j := 0; j < cnt; j++ {
				files[cur][start+j] = true
			}
		}
	}
	return files, nil
}
