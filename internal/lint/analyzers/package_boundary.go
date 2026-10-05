package analyzers

import (
	"fmt"
	"go/ast"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// PackageBoundary guards ambient effects and local adapter selection using the
// compiler's objects and the invocation's canonical Wire snapshot.
var PackageBoundary = &analysis.Analyzer{
	Name: "packageboundary", Doc: "inject production effects and select policy-free leaves in canonical Wire",
	Requires: []*analysis.Analyzer{WireSelection}, Run: runPackageBoundary,
}

func runPackageBoundary(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok || !under(unit, "pkg") || under(unit, "pkg/wire") {
		return nil, nil
	}
	choices := pass.ResultOf[pass.Analyzer.Requires[0]].(wireChoices)
	if choices.err != nil {
		pass.Reportf(pass.Files[0].Package, "package-boundary-metadata: %s; restore canonical Wire compiler inputs", choices.err)
		return nil, nil
	}
	var found []violation
	for _, file := range pass.Files {
		found = append(found, packageBoundaryFileFindings(pass, unit, file, choices)...)
	}
	reportWithBaseline(pass, unit, setOf("production-default"), countedTestPolicy(found), false, productionDefaultDebt(pass, unit))
	return nil, nil
}

func packageBoundaryFileFindings(pass *analysis.Pass, unit string, file *ast.File, choices wireChoices) []violation {
	name := serviceSource(pass, unit, file)
	if strings.HasSuffix(name, "_test.go") || ast.IsGenerated(file) {
		return nil
	}
	var found []violation
	add := func(operation, kind, symbol string, node ast.Node) {
		found = append(found, violation{rule: "production-default", importer: unit,
			importee: name + "#" + operation + "#" + kind + "#" + symbol, pos: node.Pos(),
			hint: "inject the exact effect contract and select its implementation in pkg/wire"})
	}
	if !under(unit, "pkg/root") {
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err == nil && spec.Name != nil && spec.Name.Name == "." && productionDefaultControlledImport(path) {
				add("package", "opaque-import", path, spec)
			}
		}
	}
	for _, decl := range file.Decls {
		operation := productionDefaultOperation(decl)
		ast.Inspect(decl, func(node ast.Node) bool {
			if !under(unit, "pkg/root") {
				symbol := wireQualifiedObjectExpression(node, pass)
				if kind := productionDefaultSymbols[symbol]; kind != "" && !productionDefaultPermitted(name, operation, symbol, choices) {
					add(operation, kind, symbol, node)
				}
			}
			if symbol := productionAdapterSelection(node, pass); symbol != "" {
				add(operation, "platform-adapter-selection", symbol, node)
			}
			return true
		})
	}
	return found
}

func productionAdapterSelection(node ast.Node, pass *analysis.Pass) string {
	var expression ast.Expr
	switch n := node.(type) {
	case *ast.CallExpr:
		expression = n.Fun
	case *ast.CompositeLit:
		expression = n.Type
	case *ast.SelectorExpr:
		if wireQualifiedObject(n, pass.TypesInfo) == modulePrefix+"pkg/platform/process.NewParentOwnedStdio" {
			expression = n
		}
	}
	symbol := wireQualifiedObject(expression, pass.TypesInfo)
	if _, controlled := platformAdapterSelectionSymbols[strings.TrimPrefix(symbol, modulePrefix)]; controlled {
		return symbol
	}
	return ""
}

func productionDefaultDebt(pass *analysis.Pass, unit string) map[string]struct{} {
	listed := baseline()
	selected, ignored := map[string]bool{}, map[string]bool{}
	for _, file := range pass.Files {
		selected[serviceSource(pass, unit, file)] = true
	}
	for _, name := range pass.IgnoredFiles {
		ignored[sourceName(unit, name)] = true
	}
	for key := range listed {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) != 3 || parts[0] != "production-default" || parts[1] != unit {
			continue
		}
		name, _, _ := strings.Cut(parts[2], "#")
		if !selected[name] && ignored[name] {
			delete(listed, key)
		}
	}
	return listed
}

func wireQualifiedObjectExpression(node ast.Node, pass *analysis.Pass) string {
	expression, ok := node.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return wireQualifiedObject(expression, pass.TypesInfo)
}

func productionDefaultOperation(decl ast.Decl) string {
	fn, ok := decl.(*ast.FuncDecl)
	if !ok {
		return "package"
	}
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	receiver := fn.Recv.List[0].Type
	if pointer, ok := receiver.(*ast.StarExpr); ok {
		receiver = pointer.X
	}
	if indexed, ok := receiver.(*ast.IndexExpr); ok {
		receiver = indexed.X
	}
	if indexed, ok := receiver.(*ast.IndexListExpr); ok {
		receiver = indexed.X
	}
	if identifier, ok := receiver.(*ast.Ident); ok {
		return identifier.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func productionDefaultPermitted(file, operation, symbol string, choices wireChoices) bool {
	if choices.err != nil {
		return false
	}
	for _, allowance := range productionDefaultAllowances {
		if allowance.filePath == file && allowance.operation == operation && allowance.symbol == symbol && choices.selected[modulePrefix+allowance.wireSymbol] {
			return true
		}
	}
	return false
}

func productionDefaultControlledImport(path string) bool {
	for symbol := range productionDefaultSymbols {
		if strings.HasPrefix(symbol, path+".") {
			return true
		}
	}
	return false
}

func validateProductionDefaultKey(parts []string) error {
	site, count, counted := strings.Cut(parts[2], "::count=")
	fields := strings.Split(site, "#")
	if len(fields) != 4 || !counted || !positiveDecimal(count) || !strings.HasPrefix(fields[0], parts[1]+"/") ||
		!strings.HasSuffix(fields[0], ".go") || strings.HasSuffix(fields[0], "_test.go") || strings.Contains(fields[0], "..") || fields[1] == "" {
		return fmt.Errorf("malformed production default baseline key: %s", strings.Join(parts, "|"))
	}
	kind, symbol := fields[2], fields[3]
	valid := productionDefaultSymbols[symbol] == kind && kind != ""
	if kind == "opaque-import" {
		valid = productionDefaultControlledImport(symbol)
	}
	if kind == "platform-adapter-selection" {
		_, valid = platformAdapterSelectionSymbols[strings.TrimPrefix(symbol, modulePrefix)]
	}
	if !valid {
		return fmt.Errorf("unknown production default rule: %s", strings.Join(parts, "|"))
	}
	return nil
}
