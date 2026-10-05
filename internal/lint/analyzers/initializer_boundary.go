package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Initializer's remaining boundary rules have no recorded debt. Report each
// occurrence directly so baseline entries cannot waive this zero-debt policy.
// Behavior already handles product construction and excludes generated files.
func initializerBoundaryFindings(pass *analysis.Pass, file *ast.File) {
	add := func(kind, symbol string, pos token.Pos) {
		pass.Reportf(pos, "initializer-boundary-%s: %s; inject a lifecycle-ready role instead of owning product behavior", kind, symbol)
	}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		kind := initializerImportKind(path)
		if kind != "" {
			add(kind, path, spec.Pos())
		}
	}
	application := under(strings.TrimPrefix(pass.Pkg.Path(), modulePrefix), "pkg/initializer/application")
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.TypeSpec:
			initializerTypeFinding(file.Name.Name, node, add)
		case *ast.ValueSpec:
			for _, name := range node.Names {
				if initializerModeName(name.Name) {
					add("product-lifecycle-mode", name.Name, name.Pos())
				}
			}
		case *ast.Field:
			initializerFieldFindings(pass, node, add)
			// Preserve the legacy field traversal: embedded declarations do not
			// independently count as product lifecycle types or values.
			return false
		case *ast.FuncDecl:
			initializerFunctionFinding(pass, file, node, application, add)
		case *ast.CallExpr:
			selector, ok := node.Fun.(*ast.SelectorExpr)
			if ok && application && selector.Sel.Name == "Stat" {
				// Any selected Stat operation was forbidden, including injected
				// stream methods. This is deliberately not an os-only predicate.
				add("stream-stat-fallback", "Stat", selector.Sel.Pos())
			}
		}
		return true
	})
}

func initializerImportKind(path string) string {
	rel := relImport(path)
	switch {
	case path == "net/http":
		return "http-coupling"
	case !strings.HasPrefix(path, modulePrefix):
		return ""
	case strings.HasPrefix(rel, "pkg/services/"):
		return "service-coupling"
	case strings.HasPrefix(rel, "pkg/transports/"):
		return "transport-coupling"
	case strings.HasPrefix(path, modulePrefix+"pkg/") && !under(rel, "pkg/initializer") && rel != "pkg/platform/runtimeartifact":
		return "non-lifecycle-repository-coupling"
	default:
		return ""
	}
}

func initializerTypeFinding(packageName string, spec *ast.TypeSpec, add func(string, string, token.Pos)) {
	name := spec.Name.Name
	switch {
	case packageName == "initializer" && (name == "MCPApplication" || name == "RuntimeDiagnosticsProvider"):
		add("retired-initializer-surface", name, spec.Name.Pos())
	case name == "Mode" || name == "ProcessMode":
		add("product-lifecycle-mode", name, spec.Name.Pos())
	case name == "Lifecycles":
		add("product-lifecycle-slots", name, spec.Name.Pos())
	}
}

func initializerModeName(name string) bool {
	return strings.HasPrefix(name, "ModeAPI") || strings.HasPrefix(name, "ModeCLI") ||
		strings.HasPrefix(name, "ModeMCP") || strings.HasPrefix(name, "ProcessMode")
}

var initializerProductSlots = setOf("API", "CLI", "MCP", "Runtime", "Workers", "FactoryVisualization")

func initializerFieldFindings(pass *analysis.Pass, field *ast.Field, add func(string, string, token.Pos)) {
	for _, name := range field.Names {
		if initializerProductSlots[name.Name] {
			add("product-lifecycle-slot", name.Name, name.Pos())
		}
	}
	ast.Inspect(field.Type, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj, ok := pass.TypesInfo.Uses[identifier].(*types.TypeName)
		if ok && obj.Pkg() != nil && obj.Pkg().Path() == modulePrefix+"pkg/services/edges" && obj.Name() == "Edges" {
			add("edge-bag", "pkg/services/edges.Edges", identifier.Pos())
		}
		return true
	})
}

func initializerFunctionFinding(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl, application bool, add func(string, string, token.Pos)) {
	if file.Name.Name == "initializer" && fn.Recv == nil && (fn.Name.Name == "StartSidecar" || fn.Name.Name == "RuntimeDiagnostics") {
		add("retired-initializer-surface", fn.Name.Name, fn.Name.Pos())
	}
	if !application || file.Name.Name != "application" || fn.Name.Name != "NewCommand" || fn.Recv == nil {
		return
	}
	obj, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return
	}
	receiver := obj.Type().(*types.Signature).Recv().Type()
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = pointer.Elem()
	}
	if named, ok := types.Unalias(receiver).(*types.Named); ok && named.Obj().Name() == "Process" {
		add("exported-command-construction", "Process.NewCommand", fn.Name.Pos())
	}
}
