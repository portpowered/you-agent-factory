package analyzers

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// BaselineGrowth runs once on the baseline-owning compilation unit. Git reads
// one historical object; the compiler identifies the current source directory.
// No filesystem discovery or source inventory is involved.
var BaselineGrowth = &analysis.Analyzer{
	Name: "baselinegrowth",
	Doc:  "reject growth of established exact-debt rules and missing merge-base history",
	Run:  runBaselineGrowth,
}

const baselinePath = "internal/lint/analyzers/baseline.txt"

func runBaselineGrowth(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok || unit != "internal/lint/analyzers" {
		return nil, nil
	}
	directory := filepath.Dir(pass.Fset.Position(pass.Files[0].Package).Filename)
	base, head, err := readBaselineHistory(directory)
	reportBaselineGrowth(pass, base, head, err)
	return nil, nil
}

// BaselineGrowthForDirectory snapshots external inputs before golangci consults
// its issue cache. Analyzer names participate in v2.11.4's cache key, whereas
// arbitrary Git state and non-source baseline edits do not. Rule evaluation
// still happens in Run on the compiler-selected owner.
func BaselineGrowthForDirectory(directory, configuration string) *analysis.Analyzer {
	base, head, err := readBaselineHistory(directory)
	return baselineGrowthSnapshot(base, head, err, configuration)
}

func baselineGrowthSnapshot(base, head string, err error, configuration string) *analysis.Analyzer {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%v\x00%s", base, head, err, configuration)))
	analyzer := *BaselineGrowth
	analyzer.Name = fmt.Sprintf("baselinegrowth_%x", digest[:12])
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		if unit, ok := unitKey(pass); ok && unit == "internal/lint/analyzers" {
			reportBaselineGrowth(pass, base, head, err)
		}
		return nil, nil
	}
	return &analyzer
}

func reportBaselineGrowth(pass *analysis.Pass, base, head string, err error) {
	if err == nil {
		_, err = CompareBaselineGrowth(base, head)
	}
	if err != nil {
		pass.Reportf(pass.Files[0].Package, "baseline-growth: %s", err)
	}
}

func readBaselineHistory(directory string) (string, string, error) {
	readGit := func(args ...string) (string, error) {
		command := exec.Command("git", append([]string{"-C", directory}, args...)...)
		output, err := command.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
		return string(output), nil
	}
	readHead := func(root string) (string, error) {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(baselinePath)))
		return string(data), err
	}
	return loadBaselineHistory(readGit, readHead)
}

// Boundaries keep history/IO failure cases component-isolated in unit tests.
func loadBaselineHistory(readGit func(...string) (string, error), readHead func(string) (string, error)) (string, string, error) {
	root, err := readGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	base, err := readGit("merge-base", "HEAD", "origin/main")
	if err != nil {
		return "", "", err
	}
	baseText, err := readGit("show", strings.TrimSpace(base)+":"+baselinePath)
	if err != nil {
		return "", "", fmt.Errorf("read merge-base baseline: %w", err)
	}
	headText, err := readHead(strings.TrimSpace(root))
	if err != nil {
		return "", "", fmt.Errorf("read current baseline: %w", err)
	}
	return baseText, headText, nil
}

// CompareBaselineGrowth allows the first seed of a rule absent from base.
// Established rules remain deletion-only except a single count-neutral
// interface member replacement within the same package's existing allowance.
func CompareBaselineGrowth(baseText, headText string) ([]string, error) {
	base, head := parseBaseline(baseText), parseBaseline(headText)
	rules := map[string]bool{}
	for key := range base {
		rules[strings.SplitN(key, "|", 2)[0]] = true
	}
	seeded := map[string]bool{}
	var added []string
	for key := range head {
		if _, exists := base[key]; exists {
			continue
		}
		if isInterfaceMemberReplacement(key, base, head) {
			continue
		}
		rule := strings.SplitN(key, "|", 2)[0]
		if rules[rule] {
			added = append(added, key)
		} else {
			seeded[rule] = true
		}
	}
	sort.Strings(added)
	if len(added) > 0 {
		return nil, fmt.Errorf("established baseline rules gained keys; fix the violations instead:\n%s", strings.Join(added, "\n"))
	}
	var names []string
	for rule := range seeded {
		names = append(names, rule)
	}
	sort.Strings(names)
	return names, nil
}

func isInterfaceMemberReplacement(key string, base, head map[string]struct{}) bool {
	parts := strings.Split(key, "|")
	if len(parts) != 3 || parts[0] != "service-root-interface-count" {
		return false
	}
	prefix := parts[0] + "|" + parts[1] + "|"
	previous, baseCount := interfaceAllowanceMembers(prefix, base)
	current, headCount := interfaceAllowanceMembers(prefix, head)
	if baseCount != 1 || headCount != 1 || len(previous) != len(current) {
		return false
	}
	removed := 0
	for member := range previous {
		if _, exists := current[member]; !exists {
			removed++
		}
	}
	return removed == 1
}

func interfaceAllowanceMembers(prefix string, entries map[string]struct{}) (map[string]struct{}, int) {
	var members map[string]struct{}
	count := 0
	for key := range entries {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		count++
		members = make(map[string]struct{})
		for _, member := range strings.Split(strings.TrimPrefix(key, prefix), ",") {
			if member == "" {
				return nil, 0
			}
			if _, duplicate := members[member]; duplicate {
				return nil, 0
			}
			members[member] = struct{}{}
		}
	}
	return members, count
}
