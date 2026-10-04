// Command testhomecheck is a ratchet that blocks new Go test files from
// touching the real user home profile. A test file "touches home" when it
// calls os.UserHomeDir, reads HOME/USERPROFILE, names ~/.you-agent-factory, or
// builds a factory process. It is "isolated" when its package directory uses
// testhome.IsolateHome / testhome.IsolateHomeMain, sets HOME/USERPROFILE itself,
// or passes them to a subprocess environment. Existing unisolated files are
// listed in the checked-in baseline; only new ones fail.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const defaultBaseline = "docs/internal/baselines/test-home-isolation-baseline.txt"

var scanRoots = []string{"cmd", "internal", "pkg", "tests"}

var touchMarkers = []string{
	"os.UserHomeDir(", `Getenv("HOME")`, `Getenv("USERPROFILE")`,
	".you-agent-factory", "BuildProcess(",
}

var isolationMarkers = []string{
	"testhome.IsolateHome(", "testhome.IsolateHomeMain(",
	`Setenv("HOME"`, `Setenv("USERPROFILE"`,
	`"HOME="`, `"USERPROFILE="`,
}

func main() {
	root := flag.String("root", ".", "repository root")
	baseline := flag.String("baseline", defaultBaseline, "repository-relative baseline")
	update := flag.Bool("update", false, "rewrite the baseline from the current tree")
	flag.Parse()
	if err := run(*root, *baseline, *update, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(root, baselineRel string, update bool, out io.Writer) error {
	violations, err := scan(root)
	if err != nil {
		return err
	}
	baselinePath := filepath.Join(root, filepath.FromSlash(baselineRel))
	if update {
		return writeBaseline(baselinePath, violations)
	}
	allowed, err := readBaseline(baselinePath)
	if err != nil {
		return err
	}
	current := map[string]bool{}
	var added []string
	for _, v := range violations {
		current[v] = true
		if !allowed[v] {
			added = append(added, v)
		}
	}
	if len(added) > 0 {
		return fmt.Errorf("[agent-factory:test-home-isolation] %d test file(s) touch the real user home without isolation:\n  %s\n"+
			"Call testhome.IsolateHome(t) (or testhome.IsolateHomeMain() in TestMain) in the package. "+
			"Do not add to the baseline.", len(added), strings.Join(added, "\n  "))
	}
	var stale int
	for v := range allowed {
		if !current[v] {
			stale++
		}
	}
	fmt.Fprintf(out, "[agent-factory:test-home-isolation] ok (%d baselined, %d resolved; run with -update to ratchet down)\n", len(allowed)-stale, stale)
	return nil
}

func scan(root string) ([]string, error) {
	type pkgInfo struct {
		isolated bool
		touching []string
	}
	pkgs := map[string]*pkgInfo{}
	for _, sub := range scanRoots {
		start := filepath.Join(root, sub)
		if _, err := os.Stat(start); err != nil {
			continue
		}
		err := filepath.WalkDir(start, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				if n := d.Name(); n == "testdata" || n == "node_modules" || n == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(d.Name(), "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "cmd/testhomecheck/") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(data)
			dir := filepath.ToSlash(filepath.Dir(rel))
			info := pkgs[dir]
			if info == nil {
				info = &pkgInfo{}
				pkgs[dir] = info
			}
			if containsAny(text, isolationMarkers) {
				info.isolated = true
			}
			if containsAny(text, touchMarkers) {
				info.touching = append(info.touching, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	var out []string
	for _, info := range pkgs {
		if !info.isolated {
			out = append(out, info.touching...)
		}
	}
	sort.Strings(out)
	return out, nil
}

func containsAny(text string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

func readBaseline(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open baseline: %w", err)
	}
	defer f.Close()
	set := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			set[line] = true
		}
	}
	return set, sc.Err()
}

func writeBaseline(path string, entries []string) error {
	var b strings.Builder
	b.WriteString("# Test files that touch the real user home without isolation. Ratchet: delete lines as\n" +
		"# packages adopt testhome.IsolateHome / IsolateHomeMain; never add lines. Regenerate with\n" +
		"# go run ./cmd/testhomecheck -update\n")
	for _, e := range entries {
		b.WriteString(e + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
