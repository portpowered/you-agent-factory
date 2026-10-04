package analyzers

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Testsleep preserves timing debt by file, declaration, kind and occurrence,
// rather than source line. It consumes compiler syntax and type information.
var Testsleep = &analysis.Analyzer{
	Name: "testsleep",
	Doc:  "reject new fixed sleeps, short literal deadlines and elapsed-time assertions in tests and helpers",
	Run:  runTestsleep,
}

func runTestsleep(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	var found []violation
	active := map[string]bool{}
	hasTests := false
	for _, file := range pass.Files {
		path := timingFile(unit, pass.Fset.Position(file.Pos()).Filename)
		active[path] = true
		hasTests = hasTests || strings.HasSuffix(path, "_test.go")
		if !timingScope(path) || ast.IsGenerated(file) {
			continue
		}
		found = append(found, timingFindings(pass, unit, path, file)...)
	}
	ignored := map[string]bool{}
	for _, path := range pass.IgnoredFiles {
		ignored[timingFile(unit, path)] = true
	}
	reportTiming(pass, unit, found, active, ignored, hasTests)
	return nil, nil
}

func timingFile(unit, filename string) string {
	dir := filepath.ToSlash(filepath.Dir(filename))
	// A real directory may itself end in _test. Only synthetic external-test
	// unit suffixes differ from the compiler-provided source directory.
	if !strings.HasSuffix(dir, "/"+unit) && dir != unit {
		unit = strings.TrimSuffix(unit, "_test")
	}
	return unit + "/" + filepath.Base(filename)
}

func timingScope(path string) bool {
	if strings.Contains(path, "/testdata/") {
		return false
	}
	if !under(path, "cmd") && !under(path, "internal") && !under(path, "pkg") && !under(path, "tests") {
		return false
	}
	return strings.HasSuffix(path, "_test.go") || under(path, "tests") || under(path, "internal/testutil")
}

func timingFindings(pass *analysis.Pass, unit, path string, file *ast.File) []violation {
	exempt := timingExemptions(pass, file)
	counts := map[string]int{}
	var found []violation
	for _, decl := range file.Decls {
		name := timingDeclaration(decl)
		ast.Inspect(decl, func(node ast.Node) bool {
			kind := timingKind(pass, node)
			if kind == "" {
				return true
			}
			line := pass.Fset.Position(node.Pos()).Line
			if exempt[line] || exempt[line-1] {
				return true
			}
			group := path + "::" + name + "::" + kind
			counts[group]++
			rule := "testsleep-" + kind
			if strings.HasSuffix(path, "_test.go") {
				rule += "-test"
			}
			found = append(found, violation{rule: rule, importer: unit,
				importee: fmt.Sprintf("%s::%d", group, counts[group]), pos: node.Pos()})
			return true
		})
	}
	return found
}

func timingExemptions(pass *analysis.Pass, file *ast.File) map[int]bool {
	const marker = "nolint:testsleep"
	exempt := map[int]bool{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
			if !strings.HasPrefix(text, marker) {
				continue
			}
			reason := strings.TrimSpace(strings.TrimPrefix(text, marker))
			if !strings.HasPrefix(reason, "//") || strings.TrimSpace(strings.TrimPrefix(reason, "//")) == "" {
				pass.Reportf(comment.Pos(), "//%s requires a reason: `//%s // why`", marker, marker)
				continue
			}
			exempt[pass.Fset.Position(comment.Pos()).Line] = true
		}
	}
	return exempt
}

func timingDeclaration(decl ast.Decl) string {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok {
		return "<package>"
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return timingReceiver(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func timingReceiver(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.StarExpr:
		return timingReceiver(expr.X)
	case *ast.IndexExpr:
		return timingReceiver(expr.X)
	case *ast.IndexListExpr:
		return timingReceiver(expr.X)
	case *ast.Ident:
		return expr.Name
	}
	return "?"
}

// Only inactive files are deferred to another configuration. A removed file
// or declaration in a live unit must not leave a reusable allowance behind.
func reportTiming(pass *analysis.Pass, unit string, found []violation, active, ignored map[string]bool, hasTests bool) {
	listed := baseline()
	seen := map[string]bool{}
	for _, site := range found {
		seen[site.key()] = true
		if _, allowed := listed[site.key()]; !allowed {
			pass.Reportf(site.pos, "%s: %s; wait for an event or condition, use an injected clock, or add `//nolint:testsleep // reason`", site.rule, site.key())
		}
	}
	var stale []string
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || parts[1] != unit || !strings.HasPrefix(parts[0], "testsleep-") || seen[key] {
			continue
		}
		if strings.HasSuffix(parts[0], "-test") && !hasTests {
			continue
		}
		path, _, _ := strings.Cut(parts[2], "::")
		// External test passes may omit ignored test files entirely. Compiler
		// metadata across configurations owns absent-file reconciliation.
		if !active[path] || ignored[path] {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	for _, key := range stale {
		pass.Reportf(pass.Files[0].Package, "stale baseline entry `%s` no longer reproduces; delete it from internal/lint/analyzers/baseline.txt", key)
	}
}
