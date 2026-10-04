package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	kindSleep    = "sleep"
	kindDeadline = "deadline"
	kindElapsed  = "elapsed"

	exemptionMarker = "nolint:testsleep"
	// maxShortDeadline is the longest literal duration treated as a fixed
	// deadline. Longer literals are generous ceilings.
	maxShortDeadline = 5 * time.Second
)

// siteKey identifies a baselined group of sites; code motion does not change it.
type siteKey struct {
	File     string `json:"file"`
	Function string `json:"function"`
	Kind     string `json:"kind"`
}

type baselineEntry struct {
	siteKey
	Count int `json:"count"`
}

type baselineFile struct {
	Version int             `json:"version"`
	Unit    string          `json:"countUnit"`
	Entries []baselineEntry `json:"entries"`
}

type scanResult struct {
	counts    map[siteKey]int
	lines     map[siteKey][]int
	malformed []string
}

var scanRoots = []string{"cmd", "internal", "pkg", "tests"}

func scanRepository(root string) (scanResult, error) {
	result := scanResult{counts: map[siteKey]int{}, lines: map[siteKey][]int{}}
	for _, sub := range scanRoots {
		dir := filepath.Join(root, sub)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "node_modules", "vendor", "testdata", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if !inScope(rel) {
				return nil
			}
			return scanFile(path, rel, &result)
		})
		if err != nil {
			return result, fmt.Errorf("scan %s: %w", sub, err)
		}
	}
	sort.Strings(result.malformed)
	return result, nil
}

// inScope selects test files everywhere plus non-test helpers under tests/ and
// internal/testutil.
func inScope(rel string) bool {
	if !strings.HasSuffix(rel, ".go") {
		return false
	}
	if strings.HasSuffix(rel, "_test.go") {
		return true
	}
	return strings.HasPrefix(rel, "tests/") || strings.HasPrefix(rel, "internal/testutil/")
}

func scanFile(path, rel string, result *scanResult) error {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	if ast.IsGenerated(file) {
		return nil
	}
	exempt, bad := exemptions(fset, file)
	for _, line := range bad {
		result.malformed = append(result.malformed, fmt.Sprintf("%s:%d: //%s requires a reason: `//%s // why`", rel, line, exemptionMarker, exemptionMarker))
	}
	names := importNames(file)
	record := func(fn string, pos token.Pos, kind string) {
		line := fset.Position(pos).Line
		if exempt[line] || exempt[line-1] {
			return
		}
		key := siteKey{File: rel, Function: fn, Kind: kind}
		result.counts[key]++
		result.lines[key] = append(result.lines[key], line)
	}
	for _, decl := range file.Decls {
		fn := declName(decl)
		ast.Inspect(decl, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CallExpr:
				if kind := classifyCall(n, names); kind != "" {
					record(fn, n.Pos(), kind)
				}
			case *ast.BinaryExpr:
				if isElapsedWindow(n, names) {
					record(fn, n.Pos(), kindElapsed)
				}
			}
			return true
		})
	}
	return nil
}

// exemptions returns lines carrying a valid exemption and lines with a marker
// but no reason.
func exemptions(fset *token.FileSet, file *ast.File) (map[int]bool, []int) {
	ok := map[int]bool{}
	var bad []int
	for _, group := range file.Comments {
		for _, c := range group.List {
			text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
			if !strings.HasPrefix(text, exemptionMarker) {
				continue
			}
			rest := strings.TrimSpace(strings.TrimPrefix(text, exemptionMarker))
			line := fset.Position(c.Pos()).Line
			if strings.HasPrefix(rest, "//") && strings.TrimSpace(strings.TrimPrefix(rest, "//")) != "" {
				ok[line] = true
			} else {
				bad = append(bad, line)
			}
		}
	}
	return ok, bad
}

func declName(decl ast.Decl) string {
	fn, isFunc := decl.(*ast.FuncDecl)
	if !isFunc {
		return "<package>"
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func receiverName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverName(t.X)
	case *ast.IndexExpr:
		return receiverName(t.X)
	case *ast.IndexListExpr:
		return receiverName(t.X)
	case *ast.Ident:
		return t.Name
	}
	return "?"
}

type pkgNames struct{ time, context string }

func importNames(file *ast.File) pkgNames {
	names := pkgNames{}
	for _, imp := range file.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		local := ""
		if imp.Name != nil {
			local = imp.Name.Name
		}
		switch path {
		case "time":
			names.time = pick(local, "time")
		case "context":
			names.context = pick(local, "context")
		}
	}
	return names
}

func pick(local, def string) string {
	if local == "" {
		return def
	}
	return local
}

func selector(call *ast.CallExpr) (string, string) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return id.Name, sel.Sel.Name
}

func classifyCall(call *ast.CallExpr, names pkgNames) string {
	pkg, name := selector(call)
	if pkg == "" {
		return ""
	}
	switch {
	case names.time != "" && pkg == names.time:
		switch name {
		case "Sleep":
			return kindSleep
		case "After", "NewTimer", "AfterFunc":
			if len(call.Args) > 0 && isShortDuration(call.Args[0], names) {
				return kindDeadline
			}
		}
	case names.context != "" && pkg == names.context:
		switch name {
		case "WithTimeout":
			if len(call.Args) == 2 && isShortDuration(call.Args[1], names) {
				return kindDeadline
			}
		case "WithDeadline":
			if len(call.Args) == 2 && isShortDeadline(call.Args[1], names) {
				return kindDeadline
			}
		}
	}
	return ""
}

// isShortDeadline matches time.Now().Add(<short literal>).
func isShortDeadline(expr ast.Expr, names pkgNames) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Add" && isShortDuration(call.Args[0], names)
}

func isShortDuration(expr ast.Expr, names pkgNames) bool {
	ns, ok := evalDuration(expr, names)
	return ok && ns <= int64(maxShortDeadline)
}

// isElapsedWindow matches `time.Since(x) < <literal>` style comparisons, and
// `a.Sub(b) > <literal>`.
func isElapsedWindow(bin *ast.BinaryExpr, names pkgNames) bool {
	switch bin.Op {
	case token.LSS, token.LEQ, token.GTR, token.GEQ:
	default:
		return false
	}
	return (isElapsedCall(bin.X, names) && isLiteralDuration(bin.Y, names)) ||
		(isElapsedCall(bin.Y, names) && isLiteralDuration(bin.X, names))
}

func isElapsedCall(expr ast.Expr, names pkgNames) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if pkg, name := selector(call); name == "Since" && pkg != "" && pkg == names.time {
		return true
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Sub" && len(call.Args) == 1
}

func isLiteralDuration(expr ast.Expr, names pkgNames) bool {
	_, ok := evalDuration(expr, names)
	return ok
}

var unitNanos = map[string]int64{
	"Nanosecond": 1, "Microsecond": 1e3, "Millisecond": 1e6,
	"Second": 1e9, "Minute": 60e9, "Hour": 3600e9,
}

// evalDuration folds literal duration expressions such as 2*time.Second or
// 500*time.Millisecond into nanoseconds. Anything involving a variable is not
// literal.
func evalDuration(expr ast.Expr, names pkgNames) (int64, bool) {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return evalDuration(e.X, names)
	case *ast.BasicLit:
		if e.Kind == token.INT {
			v, err := strconv.ParseInt(strings.ReplaceAll(e.Value, "_", ""), 0, 64)
			return v, err == nil
		}
		if e.Kind == token.FLOAT {
			v, err := strconv.ParseFloat(strings.ReplaceAll(e.Value, "_", ""), 64)
			return int64(v), err == nil
		}
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok && names.time != "" && id.Name == names.time {
			v, found := unitNanos[e.Sel.Name]
			return v, found
		}
	case *ast.CallExpr:
		// time.Duration(<literal>) conversion
		if len(e.Args) == 1 {
			if sel, ok := e.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Duration" {
				return evalDuration(e.Args[0], names)
			}
		}
	case *ast.BinaryExpr:
		x, okX := evalDuration(e.X, names)
		y, okY := evalDuration(e.Y, names)
		if !okX || !okY {
			return 0, false
		}
		switch e.Op {
		case token.MUL:
			return x * y, true
		case token.ADD:
			return x + y, true
		case token.SUB:
			return x - y, true
		case token.QUO:
			if y != 0 {
				return x / y, true
			}
		}
	}
	return 0, false
}

func buildBaseline(counts map[siteKey]int) baselineFile {
	out := baselineFile{Version: 1, Unit: "test-sleep-or-fixed-deadline-site", Entries: []baselineEntry{}}
	for key, count := range counts {
		out.Entries = append(out.Entries, baselineEntry{siteKey: key, Count: count})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Function != b.Function {
			return a.Function < b.Function
		}
		return a.Kind < b.Kind
	})
	return out
}

func writeBaseline(path string, baseline baselineFile) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func loadBaseline(path string) (map[siteKey]int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read baseline: %w", err)
	}
	var file baselineFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse baseline %s: %w", filepath.ToSlash(path), err)
	}
	out := make(map[siteKey]int, len(file.Entries))
	for _, entry := range file.Entries {
		out[entry.siteKey] += entry.Count
	}
	return out, nil
}

// compare reports every key whose current count exceeds its baseline count.
func compare(result scanResult, baseline map[siteKey]int) []string {
	var problems []string
	for key, count := range result.counts {
		allowed := baseline[key]
		if count <= allowed {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s: %s (%s) has %d site(s), baseline allows %d (lines %v)",
			key.File, key.Function, key.Kind, count, allowed, result.lines[key]))
	}
	sort.Strings(problems)
	return problems
}
