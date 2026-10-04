package analyzers

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Behavior keeps behavior out of the packages that must stay thin: the
// initializer must not construct product dependencies, and transports must
// not own service policy, external effects or asynchronous lifecycle. Mapping
// keeps its existing rule IDs for overlapping findings. Objects are resolved
// through go/types, so import aliases cannot hide an operation.
var Behavior = &analysis.Analyzer{
	Name: "behavior",
	Doc:  "keep initializer construction and transport behavior behind their owning service boundaries",
	Run:  runBehavior,
}

var mappingOSCalls = setOf(
	"Chmod", "Chown", "Chtimes", "Create", "CreateTemp", "FindProcess", "Lchown", "Link", "Mkdir", "MkdirAll",
	"MkdirTemp", "Open", "OpenFile", "ReadDir", "ReadFile", "Lstat", "Stat", "Remove", "RemoveAll",
	"Rename", "StartProcess", "Symlink", "Truncate", "WriteFile",
)

var mappingTimerCalls = setOf("After", "AfterFunc", "NewTicker", "NewTimer", "Sleep", "Tick")

var constructionPrefixes = []string{"Build", "Create", "Inject", "New", "Open", "Provide"}

var behaviorRuleNames = func() map[string]bool {
	names := map[string]bool{}
	for _, rule := range []string{
		"initializer-product-construction", "mapping-filesystem-behavior", "mapping-process-behavior",
		"mapping-timer-behavior", "mapping-goroutine", "functional-configuration",
	} {
		names[rule] = true
		names[rule+"-test"] = true
	}
	for _, kind := range transportBehaviorKinds {
		names["transport-"+kind] = true
		names["transport-"+kind+"-test"] = true
	}
	return names
}()

func setOf(items ...string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, item := range items {
		set[item] = true
	}
	return set
}

func constructionName(name string) bool {
	for _, prefix := range constructionPrefixes {
		if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
			return true
		}
	}
	return false
}

func runBehavior(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	importer := strings.TrimSuffix(unit, "_test")
	initializer := under(importer, "pkg/initializer")
	mapping := under(importer, "pkg/transports/mapping")
	functional := functionalSource(importer)
	transport := under(importer, "pkg/transports")
	if !initializer && !transport && !functional {
		return nil, nil
	}
	var found []violation
	hasTests := false
	for _, file := range pass.Files {
		if ast.IsGenerated(file) {
			continue
		}
		test := strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
		hasTests = hasTests || test
		add := func(rule, target, hint string, pos token.Pos) {
			if test {
				rule += "-test"
			}
			found = append(found, violation{rule: rule, importer: unit, importee: target, pos: pos, hint: hint})
		}
		if functional {
			functionalConfigurationFindings(file, add)
		}
		if initializer {
			initializerFindings(pass, file, add)
			if !test {
				initializerBoundaryFindings(pass, file)
			}
		}
		if mapping {
			mappingFindings(pass, file, add)
		}
		if transport {
			transportFindings(pass, file, importer, test, add)
		}
	}
	reportAgainstBaseline(pass, unit, behaviorRuleNames, found, hasTests)
	return nil, nil
}

type addFinding func(rule, target, hint string, pos token.Pos)

func relImport(path string) string { return strings.TrimPrefix(path, modulePrefix) }

func productImport(path string) bool {
	rel := relImport(path)
	return strings.HasPrefix(rel, "pkg/services/") || strings.HasPrefix(rel, "pkg/transports/")
}

func initializerFindings(pass *analysis.Pass, file *ast.File, add addFinding) {
	const hint = "construct the product dependency in pkg/wire and inject its exact operation into the initializer"
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err == nil && spec.Name != nil && spec.Name.Name == "." && productImport(path) {
			add("initializer-product-construction", relImport(path)+".*", hint, spec.Pos())
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		pkgPath, name, ok := packageCall(pass, call)
		if ok && productImport(pkgPath) && constructionName(name) {
			add("initializer-product-construction", relImport(pkgPath)+"."+name, hint, call.Pos())
		}
		return true
	})
}

func mappingFindings(pass *analysis.Pass, file *ast.File, add addFinding) {
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if path == "os/exec" {
			add("mapping-process-behavior", "os/exec", "move process discovery or execution behind an injected service-owned port", spec.Pos())
		}
		if spec.Name != nil && spec.Name.Name == "." {
			switch path {
			case "os":
				add("mapping-filesystem-behavior", "os.*", "move filesystem behavior behind an injected service or exact external-effect port", spec.Pos())
			case "time":
				add("mapping-timer-behavior", "time.*", "move timer lifecycle behind an injected clock or owning service", spec.Pos())
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if statement, ok := node.(*ast.GoStmt); ok {
			add("mapping-goroutine", "go statement", "move asynchronous lifecycle to an owning service or initializer lifecycle component", statement.Pos())
			return true
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		pkgPath, name, ok := packageCall(pass, call)
		if !ok {
			return true
		}
		switch {
		case pkgPath == "os" && mappingOSCalls[name]:
			add("mapping-filesystem-behavior", "os."+name, "move filesystem behavior behind an injected service or exact external-effect port", call.Pos())
		case pkgPath == "os/exec":
			add("mapping-process-behavior", "os/exec."+name, "move process discovery or execution behind an injected service-owned port", call.Pos())
		case pkgPath == "time" && mappingTimerCalls[name]:
			add("mapping-timer-behavior", "time."+name, "move timer lifecycle behind an injected clock or owning service", call.Pos())
		}
		return true
	})
}

// packageCall resolves a call of the form pkg.Name(...) to the imported
// package path and function name, whatever the import alias is.
func packageCall(pass *analysis.Pass, call *ast.CallExpr) (pkgPath, name string, ok bool) {
	selector, isSelector := call.Fun.(*ast.SelectorExpr)
	if !isSelector {
		return "", "", false
	}
	qualifier, isIdent := selector.X.(*ast.Ident)
	if !isIdent {
		return "", "", false
	}
	pkgName, isPkg := pass.TypesInfo.Uses[qualifier].(*types.PkgName)
	if !isPkg {
		return "", "", false
	}
	return pkgName.Imported().Path(), selector.Sel.Name, true
}
