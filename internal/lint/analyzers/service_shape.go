package analyzers

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// ServiceShape checks declarations and placement in compiler-supplied service
// packages. Empty directories and missing internal directories are not rules.
// Registration and legacy-debt migration follow the shared seed-gate rollout.
var ServiceShape = &analysis.Analyzer{
	Name: "serviceshape",
	Doc:  "enforce compiler-visible service root declarations and package placement",
	Run:  runServiceShape,
}

var serviceShapeRules = setOf(
	"service-root-interface-count", "service-root-exported-function",
	"service-root-unexpected-directory", "service-container-go-file",
)

// serviceLocation classifies recursive roots using only the compiled import
// path. A public services/ container remains forbidden even while its nested
// roots are checked, so old placement debt retains its exact child target.
func serviceLocation(unit string) (root string, container bool, unexpected []string) {
	if !strings.HasPrefix(unit, "pkg/services/") {
		return "", false, nil
	}
	parts := strings.Split(unit, "/")
	root = strings.Join(parts[:3], "/")
	parts = parts[3:]
	for len(parts) > 0 {
		switch parts[0] {
		case "internal":
			if len(parts) < 2 || parts[1] != "services" {
				return "", false, unexpected
			}
			if len(parts) == 2 {
				return root, true, unexpected
			}
			root += "/internal/services/" + parts[2]
			parts = parts[3:]
		case "services":
			unexpected = append(unexpected, root+"/services")
			if len(parts) == 1 {
				return root, true, unexpected
			}
			root += "/services/" + parts[1]
			parts = parts[2:]
		case "wire", "transports":
			return "", false, unexpected
		default:
			unexpected = append(unexpected, root+"/"+parts[0])
			return "", false, unexpected
		}
	}
	return root, false, unexpected
}

func runServiceShape(pass *analysis.Pass) (any, error) {
	unit, ok := unitKey(pass)
	if !ok {
		return nil, nil
	}
	// External test units declare no production service contract, but their
	// files can still violate the container and compiled-placement rules.
	root, container, unexpected := serviceLocation(strings.TrimSuffix(unit, "_test"))
	var found []violation
	add := func(rule, target, hint string, pos token.Pos) {
		found = append(found, violation{rule: rule, importer: unit, importee: target, pos: pos, hint: hint})
	}
	for _, target := range unexpected {
		add("service-root-unexpected-directory", target,
			"move this compiled package under wire, internal, or transports", pass.Files[0].Package)
	}
	if container {
		for _, file := range pass.Files {
			name := serviceSource(pass, unit, file)
			add("service-container-go-file", name,
				"move this Go file into a named subservice below "+root+"/internal/services", file.Package)
		}
	} else if root != "" {
		found = append(found, serviceDeclarations(pass, unit)...)
	}
	reportWithBaseline(pass, unit, serviceShapeRules, found, true, serviceShapeBaseline(pass, unit))
	return nil, nil
}

func serviceSource(pass *analysis.Pass, unit string, file *ast.File) string {
	return strings.TrimSuffix(unit, "_test") + "/" + filepath.Base(pass.Fset.Position(file.Pos()).Filename)
}

func serviceDeclarations(pass *analysis.Pass, unit string) []violation {
	var interfaces []string
	var found []violation
	var first token.Pos
	for _, file := range pass.Files {
		name := serviceSource(pass, unit, file)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		if first == token.NoPos {
			first = file.Package
		}
		if ast.IsGenerated(file) {
			continue
		}
		declared, exports := serviceFileDeclarations(unit, name, file)
		interfaces = append(interfaces, declared...)
		found = append(found, exports...)
	}
	if first != token.NoPos && len(interfaces) != 1 {
		sort.Strings(interfaces)
		target := "<none>"
		if len(interfaces) > 0 {
			target = strings.Join(interfaces, ",")
		}
		found = append(found, violation{
			rule: "service-root-interface-count", importer: unit, importee: target, pos: first,
			hint: "reduce the handwritten production service root to exactly one named interface",
		})
	}
	return found
}

func serviceFileDeclarations(unit, name string, file *ast.File) ([]string, []violation) {
	var interfaces []string
	var exports []violation
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil && ast.IsExported(decl.Name.Name) {
				exports = append(exports, violation{
					rule: "service-root-exported-function", importer: unit,
					importee: name + "#" + decl.Name.Name, pos: decl.Pos(),
					hint: "move this operation behind the service interface or into internal implementation",
				})
			}
		case *ast.GenDecl:
			if decl.Tok != token.TYPE {
				continue
			}
			for _, spec := range decl.Specs {
				spec := spec.(*ast.TypeSpec)
				if _, ok := spec.Type.(*ast.InterfaceType); ok {
					interfaces = append(interfaces, name+":"+spec.Name.Name)
				}
			}
		}
	}
	return interfaces, exports
}

// serviceShapeBaseline copies the ledger and defers stale judgments for exact
// declaration targets belonging to compiler-excluded sources. No omitted file
// is parsed. The configuration selecting that source judges it instead.
func serviceShapeBaseline(pass *analysis.Pass, unit string) map[string]struct{} {
	ignored := map[string]bool{}
	for _, name := range pass.IgnoredFiles {
		ignored[strings.TrimSuffix(unit, "_test")+"/"+filepath.Base(name)] = true
	}
	listed := map[string]struct{}{}
	for key := range baseline() {
		parts := strings.SplitN(key, "|", 3)
		if len(parts) == 3 && parts[1] == unit && serviceExcludedTarget(parts[0], parts[2], ignored) {
			continue
		}
		listed[key] = struct{}{}
	}
	return listed
}

func serviceExcludedTarget(rule, target string, ignored map[string]bool) bool {
	switch rule {
	case "service-root-interface-count":
		for _, declaration := range strings.Split(target, ",") {
			name, _, _ := strings.Cut(declaration, ":")
			if ignored[name] {
				return true
			}
		}
	case "service-root-exported-function":
		name, _, _ := strings.Cut(target, "#")
		return ignored[name]
	case "service-container-go-file":
		return ignored[target]
	}
	return false
}
