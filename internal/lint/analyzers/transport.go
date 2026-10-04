package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Transport policy runs only on top-level adapters, never service-owned adapters.
// Declaration shapes use the existing AST; operation ownership uses typed objects.
func transportFindings(pass *analysis.Pass, file *ast.File, importer string, test bool, add addFinding) {
	record := func(kind, target string, pos token.Pos) {
		add("transport-"+kind, target, "move policy or lifecycle to its owning service and inject the exact role", pos)
	}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err == nil && spec.Name != nil && spec.Name.Name == "." && transportControlledImport(path) {
			record("opaque-import", relImport(path), spec.Pos())
		}
	}
	if !test {
		transportDeclarations(pass, file, importer, record)
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if !test {
			transportConcurrency(pass, node, importer, record)
			transportLifecycle(pass, node, importer, record)
		}
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil || obj.Pkg() == nil {
			return true
		}
		transportObjectPolicy(obj, importer, test, id.Pos(), record)
		return true
	})
}

type transportRecorder func(kind, target string, pos token.Pos)

func transportLifecycle(pass *analysis.Pass, node ast.Node, importer string, record transportRecorder) {
	var expression ast.Expr
	switch node := node.(type) {
	case *ast.CallExpr:
		expression = node.Fun
	case *ast.CompositeLit:
		expression = node.Type
	default:
		return
	}
	var id *ast.Ident
	switch expr := expression.(type) {
	case *ast.SelectorExpr:
		id = expr.Sel
	case *ast.Ident:
		id = expr
	default:
		return
	}
	obj := pass.TypesInfo.Uses[id]
	if obj == nil || obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return
	}
	path, name := obj.Pkg().Path(), obj.Name()
	if _, prohibited := transportLifecycleSymbols[path+"."+name]; prohibited &&
		!(under(importer, "pkg/transports/mapping") && path == "time" && mappingTimerCalls[name]) {
		record("lifecycle", path+"."+name, id.Pos())
	}
}

func transportControlledImport(path string) bool {
	return setOf("context", "net", "net/http", "os", "os/exec", "os/signal", "sync", "time")[path] ||
		under(relImport(path), "pkg/platform") || transportServiceRoot(relImport(path))
}

func transportServiceRoot(path string) bool {
	return strings.HasPrefix(path, "pkg/services/") && len(strings.Split(path, "/")) == 3
}

func transportExportedPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if name == prefix || strings.HasPrefix(name, prefix) && len(name) > len(prefix) && name[len(prefix)] >= 'A' && name[len(prefix)] <= 'Z' {
			return true
		}
	}
	return false
}

func transportConcurrency(pass *analysis.Pass, node ast.Node, importer string, record transportRecorder) {
	if statement, ok := node.(*ast.GoStmt); ok && !under(importer, "pkg/transports/mapping") {
		record("concurrency", "go", statement.Pos())
	}
	call, ok := node.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 {
		return
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || builtin.Name() != "make" {
		return
	}
	if _, channel := pass.TypesInfo.TypeOf(call.Args[0]).Underlying().(*types.Chan); channel {
		record("concurrency", "make(chan)", call.Pos())
	}
}

func transportObjectPolicy(obj types.Object, importer string, test bool, pos token.Pos, record transportRecorder) {
	path, name := relImport(obj.Pkg().Path()), obj.Name()
	qualified := path + "." + name
	mapping := under(importer, "pkg/transports/mapping")
	// Existing mapping rule IDs retain ownership of overlapping observations.
	if !test {
		_, effect := transportProcessAndFilesystemSymbols[qualified]
		if effect && !(mapping && (path == "os/exec" || path == "os" && mappingOSCalls[name])) {
			record("external-effect", qualified, pos)
		}
		if _, sync := transportSynchronizationTypes[qualified]; sync {
			record("concurrency", qualified, pos)
		}
		if obj.Parent() == obj.Pkg().Scope() && under(path, "pkg/platform") && transportExportedPrefix(name, transportPlatformSelectionPrefixes) {
			record("platform-selection", qualified, pos)
		}
		if obj.Parent() == obj.Pkg().Scope() && transportServiceRoot(path) && transportExportedPrefix(name, transportDomainPolicyPrefixes) {
			record("domain-policy", qualified, pos)
		}
		transportNamedPolicy(obj, importer, pos, record)
	}
	transportPreparationPolicy(path, name, mapping, pos, record)
}

func transportPreparationPolicy(path, name string, mapping bool, pos token.Pos, record transportRecorder) {
	// Local injected preparation interfaces are also forbidden inside mappers:
	// their operation's purpose, rather than a concrete implementation, owns policy.
	if _, prohibited := transportFactorySessionPreparationMethods[name]; mapping && prohibited {
		record("factory-session-preparation", name, pos)
	}
	if name == "NormalizeWorkRequest" {
		record("work-request-normalization", name, pos)
	}
	tables := []struct {
		path, kind string
		names      map[string]struct{}
		enabled    bool
	}{
		{"pkg/services/factory_sessions", "service-normalization", transportFactorySessionNormalizationSymbols, true},
		{"pkg/services/work", "work-request-preparation", transportWorkPreparationSymbols, true},
		{"pkg/services/workers", "workers-config-loading", transportWorkersConfigLoadingSymbols, true},
		{"pkg/services/workers", "workers-failure-policy", transportWorkersFailurePolicySymbols, mapping},
		{"pkg/services/models", "models-readiness-policy", transportModelsReadinessPolicySymbols, mapping},
	}
	for _, table := range tables {
		if _, prohibited := table.names[name]; table.enabled && path == table.path && prohibited {
			record(table.kind, path+"."+name, pos)
		}
	}
	if path == "pkg/services/factory_definitions" && name == "MapDir" {
		record("named-factory-path-policy", path+"."+name, pos)
	}
}

func transportNamedPolicy(obj types.Object, importer string, pos token.Pos, record transportRecorder) {
	path, name := relImport(obj.Pkg().Path()), obj.Name()
	if name == "DefaultSourceContext" {
		record("source-default-selection", name, pos)
	}
	if _, prohibited := transportFactoryDefinitionValidationPolicySymbols[name]; prohibited && (path == importer || path == "pkg/services/factory_definitions") {
		record("validation-orchestration", name, pos)
	}
	if _, prohibited := httpFactoryStatusProjectionSymbols[name]; prohibited && under(importer, "pkg/transports/http") && (path == importer || path == "pkg/services/factory_visualization") {
		record("factory-status-projection", name, pos)
	}
	if name == "Normalized" && path == "pkg/services/work" && under(importer, "pkg/transports/mapping/workcontent") {
		record("work-content-normalization", name, pos)
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return
	}
	signature, ok := fn.Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return
	}
	receiver := signature.Recv().Type()
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = pointer.Elem()
	}
	named, ok := types.Unalias(receiver).(*types.Named)
	if _, prohibited := transportFactoryDefinitionValidationPhaseMethods[name]; prohibited && ok && path == "pkg/services/factory_definitions" && named.Obj().Name() == "Validator" {
		record("validation-orchestration", name, pos)
	}
}

func transportDeclarations(pass *analysis.Pass, file *ast.File, importer string, record transportRecorder) {
	mcp := under(importer, strings.TrimSuffix(factorySessionMCPTransportPrefix, "/"))
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			name := fn.Name.Name
			if _, retired := retiredTransportServiceEntrypoints[name]; retired {
				record("alternate-service-entrypoint", name, fn.Name.Pos())
			}
			if _, prohibited := transportFactoryDefinitionValidationPolicySymbols[name]; prohibited {
				record("validation-orchestration", name, fn.Name.Pos())
			}
			if _, prohibited := httpFactoryStatusProjectionSymbols[name]; prohibited && under(importer, "pkg/transports/http") {
				record("factory-status-projection", name, fn.Name.Pos())
			}
			if mcp && strings.HasPrefix(name, "NewClient") {
				record("alternate-test-entrypoint", name, fn.Name.Pos())
			}
			if target := transportForwarder(pass, fn); target != "" {
				record("service-forwarder", target, fn.Name.Pos())
			}
			continue
		}
		general, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, raw := range general.Specs {
			if spec, ok := raw.(*ast.TypeSpec); ok && mcp && spec.Name.Name == "Client" {
				record("alternate-test-entrypoint", "Client", spec.Pos())
			}
			spec, ok := raw.(*ast.ValueSpec)
			if !ok || general.Tok != token.VAR {
				continue
			}
			for _, value := range spec.Values {
				if _, function := value.(*ast.FuncLit); function {
					record("mutable-function-seam", spec.Names[0].Name, value.Pos())
				}
			}
		}
	}
}

func transportForwarder(pass *analysis.Pass, fn *ast.FuncDecl) string {
	if fn.Recv != nil || !fn.Name.IsExported() || fn.Body == nil || len(fn.Body.List) != 1 {
		return ""
	}
	returned, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return ""
	}
	call, ok := returned.Results[0].(*ast.CallExpr)
	if !ok {
		return ""
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	for _, field := range fn.Type.Params.List {
		named, ok := types.Unalias(pass.TypesInfo.TypeOf(field.Type)).(*types.Named)
		if !ok || named.Obj().Pkg() == nil || !transportServiceRoot(relImport(named.Obj().Pkg().Path())) {
			continue
		}
		for _, parameter := range field.Names {
			if pass.TypesInfo.Defs[parameter] == pass.TypesInfo.Uses[receiver] {
				return fn.Name.Name + "->" + relImport(named.Obj().Pkg().Path()) + "." + selector.Sel.Name
			}
		}
	}
	return ""
}
