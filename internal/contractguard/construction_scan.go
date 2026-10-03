package contractguard

import (
	"fmt"
	"go/ast"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

type ConstructionFinding struct {
	FilePath      string
	Line          int
	Caller        ConstructionSymbol
	Callee        ConstructionSymbol
	CapabilitySet string
	Mode          ConstructionMode
	Rule          string
}

// ScanRepositoryConstruction retains sparse fixture compatibility: fixtures
// without go.mod have no repository registry. Real modules validate all entries.
func ScanRepositoryConstruction(root string) ([]ConstructionFinding, error) {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); os.IsNotExist(err) {
		return nil, nil
	}
	return ScanConstruction(root, RepositoryConstructionRegistry())
}

// ScanConstruction is the shared checker seam for qualified observations. It
// reports direct calls and constructor references; provenance rules extend it
// separately. Existing checkers continue to own their prior blocking policy.
func ScanConstruction(root string, registry ConstructionRegistry) ([]ConstructionFinding, error) {
	index, err := loadConstructionIndex(root)
	if err != nil {
		return nil, err
	}
	if err := index.validate(registry); err != nil {
		return nil, err
	}
	findings := index.scanRequiredConstructionGuards(registry)
	findings = append(findings, index.scanConstructionDependencyBags(registry)...)
	getters := index.constructionServiceGetters(registry)
	for _, source := range index.sources {
		for _, decl := range source.file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok {
				caller := ConstructionSymbol{ImportPath: source.importPath, Name: "<package>"}
				findings = append(findings, scanConstructionOperation(source, caller, decl, registry)...)
				findings = append(findings, scanConstructionServiceGetters(source, caller, decl, getters, registry)...)
				continue
			}
			caller := ConstructionSymbol{ImportPath: source.importPath, Name: function.Name.Name}
			if function.Recv != nil {
				caller.Receiver = constructionReceiver(function.Recv.List[0].Type)
			}
			if function.Body != nil {
				findings = append(findings, scanConstructionOperation(source, caller, function.Body, registry)...)
				findings = append(findings, scanConstructionServiceGetters(source, caller, function.Body, getters, registry)...)
			}
		}
	}
	slices.SortFunc(findings, func(a, b ConstructionFinding) int {
		if order := strings.Compare(a.FilePath, b.FilePath); order != 0 {
			return order
		}
		if a.Line != b.Line {
			return a.Line - b.Line
		}
		return strings.Compare(a.Callee.String(), b.Callee.String())
	})
	return findings, nil
}

func scanConstructionOperation(source *constructionSource, caller ConstructionSymbol, body ast.Node, registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	if body == nil {
		return findings
	}
	callees := make(map[ast.Expr]bool)
	indirectCalls := constructionIndirectProviderCalls(body)
	recursiveProvider := constructionRecursiveProvider(source, caller, registry.Allowances)
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		markConstructionCallee(callees, call.Fun)
		symbol, resolved := resolveConstructionCall(call.Fun, source)
		if !resolved {
			return true
		}
		for _, constructor := range registry.Constructors {
			if constructor.Symbol != symbol || !constructionProhibitedKind(constructor, registry.Types) || constructionCallAllowed(registry.Allowances, caller, symbol, source.path, indirectCalls[call] || recursiveProvider) {
				continue
			}
			mode := ConstructionReport
			for _, set := range registry.CapabilitySets {
				if set.Name == constructor.CapabilitySet {
					mode = set.Mode
				}
			}
			findings = append(findings, ConstructionFinding{
				FilePath: source.path, Line: source.set.Position(call.Pos()).Line,
				Caller: caller, Callee: symbol, CapabilitySet: constructor.CapabilitySet, Mode: mode, Rule: "registered-construction",
			})
		}
		return true
	})
	for expression := range constructionSafeValueReferences(body, source, callees) {
		callees[expression] = true
	}
	findings = append(findings, constructionReferenceDebt(source, caller, body, callees, registry)...)
	return findings
}

func constructionReferenceDebt(source *constructionSource, caller ConstructionSymbol, body ast.Node, callees map[ast.Expr]bool, registry ConstructionRegistry) []ConstructionFinding {
	var findings []ConstructionFinding
	ast.Inspect(body, func(node ast.Node) bool {
		expr, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		_, selector := expr.(*ast.SelectorExpr)
		if callees[expr] {
			return !selector
		}
		symbol, resolved := resolveConstructionCall(expr, source)
		if !resolved {
			return !selector
		}
		for _, constructor := range registry.Constructors {
			if constructor.Symbol != symbol || !constructionProhibitedKind(constructor, registry.Types) {
				continue
			}
			mode := ConstructionReport
			for _, set := range registry.CapabilitySets {
				if set.Name == constructor.CapabilitySet {
					mode = set.Mode
				}
			}
			findings = append(findings, ConstructionFinding{
				FilePath: source.path, Line: source.set.Position(expr.Pos()).Line, Caller: caller, Callee: symbol,
				CapabilitySet: constructor.CapabilitySet, Mode: mode, Rule: "unresolved-construction-reference",
			})
		}
		return !selector
	})
	return findings
}

func constructionProhibitedKind(constructor ConstructionConstructor, types []ConstructionType) bool {
	return slices.ContainsFunc(types, func(typ ConstructionType) bool {
		return slices.Contains(constructor.Results, typ.Symbol) && (typ.Kind == ConstructionBehavior || typ.Kind == ConstructionEffect)
	})
}

func resolveConstructionCall(expr ast.Expr, source *constructionSource) (ConstructionSymbol, bool) {
	return resolveConstructionValue(expr, source, map[*ast.Object]bool{})
}

func resolveConstructionValue(expr ast.Expr, source *constructionSource, visited map[*ast.Object]bool) (ConstructionSymbol, bool) {
	switch value := expr.(type) {
	case *ast.Ident:
		if value.Obj == nil && source.imports[value.Name] != "" {
			return ConstructionSymbol{}, false
		}
		if value.Obj != nil && value.Obj.Kind != ast.Fun {
			if visited[value.Obj] || source.mutations[value.Obj] {
				return ConstructionSymbol{}, false
			}
			visited[value.Obj] = true
			return resolveConstructionValue(constructionValueInitializer(value.Obj), source, visited)
		}
		local := ConstructionSymbol{ImportPath: source.importPath, Name: value.Name}
		if value.Obj != nil || source.declarations[local].function != nil {
			return local, true
		}
		for _, imported := range source.dotImports {
			dotted := ConstructionSymbol{ImportPath: imported, Name: value.Name}
			if source.declarations[dotted].function != nil {
				return dotted, true
			}
		}
		return local, true
	case *ast.SelectorExpr:
		if ident, ok := value.X.(*ast.Ident); ok && ident.Obj == nil {
			if imported := source.imports[ident.Name]; imported != "" {
				return ConstructionSymbol{ImportPath: imported, Name: value.Sel.Name}, true
			}
		}
		return resolveConstructionMethod(value, source, visited)
	case *ast.ParenExpr:
		return resolveConstructionValue(value.X, source, visited)
	case *ast.IndexExpr:
		return resolveConstructionValue(value.X, source, visited)
	case *ast.IndexListExpr:
		return resolveConstructionValue(value.X, source, visited)
	}
	return ConstructionSymbol{}, false
}

func markConstructionCallee(callees map[ast.Expr]bool, expr ast.Expr) {
	callees[expr] = true
	switch value := expr.(type) {
	case *ast.ParenExpr:
		markConstructionCallee(callees, value.X)
	case *ast.IndexExpr:
		markConstructionCallee(callees, value.X)
	case *ast.IndexListExpr:
		markConstructionCallee(callees, value.X)
	}
}

func constructionCallAllowed(allowances []ConstructionAllowance, caller, callee ConstructionSymbol, file string, indirect bool) bool {
	return slices.ContainsFunc(allowances, func(a ConstructionAllowance) bool {
		return a.Caller == caller && a.Callee == callee && a.FilePath == file && !(indirect && a.Kind == "focused-provider")
	})
}

func (index constructionIndex) validateAllowances(allowances []ConstructionAllowance, constructors map[ConstructionSymbol]bool) error {
	seen := make(map[string]bool)
	for _, allowance := range allowances {
		key := allowance.FilePath + "|" + allowance.Caller.String() + "|" + allowance.Callee.String()
		if seen[key] {
			return index.metadataError(allowance.Caller, "duplicate allowance")
		}
		seen[key] = true
		if !validConstructionSymbol(allowance.Caller) || !validConstructionSymbol(allowance.Callee) || !constructors[allowance.Callee] ||
			allowance.FilePath == "." || path.IsAbs(allowance.FilePath) || path.Clean(allowance.FilePath) != allowance.FilePath ||
			strings.ContainsAny(allowance.FilePath, "*?\\:") || strings.HasPrefix(allowance.FilePath, "../") ||
			strings.TrimSpace(allowance.OwnerTask) == "" || strings.TrimSpace(allowance.Reason) == "" {
			return index.metadataError(allowance.Caller, "invalid exact allowance")
		}
		switch allowance.Kind {
		case "focused-provider", "boundary-normalization", "leaf-effect", "scoped-view":
		default:
			return index.metadataError(allowance.Caller, "invalid allowance kind")
		}
		decl := index.declarations[allowance.Caller]
		if decl.function == nil || decl.function.Body == nil || decl.source.path != allowance.FilePath {
			return index.metadataError(allowance.Caller, "stale allowance caller/path")
		}
		found := false
		ast.Inspect(decl.function.Body, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				symbol, resolved := resolveConstructionCall(call.Fun, decl.source)
				found = found || (resolved && symbol == allowance.Callee)
			}
			return true
		})
		if !found {
			return index.metadataError(allowance.Caller, "stale allowance callee")
		}
	}
	return nil
}

func WriteConstructionFindings(writer io.Writer, findings []ConstructionFinding) {
	for _, finding := range findings {
		fmt.Fprintf(writer, "[agent-factory:construction] %s:%d caller=%s callee=%s set=%s mode=%s rule=%s\n",
			finding.FilePath, finding.Line, finding.Caller, finding.Callee, finding.CapabilitySet, finding.Mode, finding.Rule)
	}
}

func CountBlockingConstructionFindings(findings []ConstructionFinding) int {
	count := 0
	for _, finding := range findings {
		if finding.Mode == ConstructionEnforce {
			count++
		}
	}
	return count
}
