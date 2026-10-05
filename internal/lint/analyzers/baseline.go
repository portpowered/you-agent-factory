// Package analyzers holds the repository's custom go/analysis analyzers. They
// run per compilation unit through the supported golangci module plugin;
// none of them walks or greps source files.
package analyzers

import (
	_ "embed"
	"fmt"
	"go/token"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// baselineText is the checked-in exact violation list. One violation per line
// as `rule|importer|importee`; `#` starts a comment. CI refuses growth by
// comparing this one file with its merge-base version (baselinegrowth analyzer).
//
//go:embed baseline.txt
var baselineText string

// modulePrefix is the import-path prefix of this repository. Tests override it.
var modulePrefix = "github.com/portpowered/infinite-you/"

// baseline returns the listed violations. Tests replace it.
var baseline = func() map[string]struct{} { return parseBaseline(baselineText) }

func parseBaseline(text string) map[string]struct{} {
	entries := map[string]struct{}{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		entries[line] = struct{}{}
	}
	return entries
}

// violation is one observed rule breach: a from/to edge or, for behavior
// rules, a (package, target) pair.
type violation struct {
	rule     string
	importer string
	importee string
	pos      token.Pos
	hint     string
}

func (v violation) key() string { return v.rule + "|" + v.importer + "|" + v.importee }

// reportAgainstBaseline fails on violations missing from the baseline and on
// baseline entries of this unit that no longer reproduce. unit is the verbatim
// unit key (the package path relative to the module, `_test` kept for external
// test packages); rules is the set of rule names the calling analyzer owns;
// hasTests says whether this unit contains test files, which decides whether
// test-class entries can be judged stale here.
func reportAgainstBaseline(pass *analysis.Pass, unit string, rules map[string]bool, found []violation, hasTests bool) {
	reportWithBaseline(pass, unit, rules, found, hasTests, baseline())
}

// reportWithBaseline permits declaration rules to exclude debt tied to sources
// the compiler omitted in this configuration, without changing shared state.
func reportWithBaseline(pass *analysis.Pass, unit string, rules map[string]bool, found []violation, hasTests bool, listed map[string]struct{}) {
	seen := map[string]struct{}{}
	sort.Slice(found, func(i, j int) bool { return found[i].key() < found[j].key() })
	for _, v := range found {
		if _, dup := seen[v.key()]; dup {
			continue
		}
		seen[v.key()] = struct{}{}
		if _, ok := listed[v.key()]; ok {
			continue
		}
		pass.Report(analysis.Diagnostic{
			Pos:     v.pos,
			Message: fmt.Sprintf("%s: %s -> %s; %s (exact key `%s`; only newly migrated rule IDs may seed authorized debt)", v.rule, v.importer, v.importee, v.hint, v.key()),
		})
	}
	if flag := pass.Analyzer.Flags.Lookup("check-stale"); flag != nil && flag.Value.String() == "false" {
		return
	}
	var stale []string
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || !rules[parts[0]] || parts[1] != unit {
			continue
		}
		if strings.HasSuffix(parts[0], "-test") && !hasTests {
			continue
		}
		if _, ok := seen[key]; !ok {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		pass.Report(analysis.Diagnostic{
			Pos:     pass.Files[0].Package,
			Message: fmt.Sprintf("stale baseline entry `%s` no longer reproduces; delete it from internal/lint/analyzers/baseline.txt", key),
		})
	}
}

// unitKey returns the module-relative package path, or false for packages
// outside the module and for generated test-main packages.
func unitKey(pass *analysis.Pass) (string, bool) {
	path := pass.Pkg.Path()
	if !strings.HasPrefix(path, modulePrefix) {
		return "", false
	}
	path = strings.TrimPrefix(path, modulePrefix)
	if strings.HasSuffix(path, ".test") || len(pass.Files) == 0 {
		return "", false
	}
	return path, true
}

func under(path, root string) bool { return path == root || strings.HasPrefix(path, root+"/") }
