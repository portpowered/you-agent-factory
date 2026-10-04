package analyzers

import (
	"go/ast"
	"strings"
)

func functionalSource(path string) bool {
	return under(path, "tests/functional") && !containsSegment(path, "testdata")
}

func dedicatedProvider(path string) bool {
	return strings.HasPrefix(path, "tests/functional/providers/") && functionalSource(path)
}

func providerLocalSupport(path string) bool {
	return under(path, "tests/functional/providers/support") || under(path, "tests/functional/providers/internal/support")
}

// These exact legacy ports are preserved; internal visibility remains owned
// by the compiler. A descendant of an allowed port is not another public port.
var functionalProviderPorts = setOf(
	"pkg/services/providers/execution/inferencecontract",
	"pkg/services/providers/internal/services/execution/internal/adapters/agy/agypty",
	"pkg/services/providers/wire",
	"pkg/services/automations",
)

func violatesFunctionalProvider(e edge) bool {
	if !dedicatedProvider(e.importer) {
		return false
	}
	if e.importee == "pkg/root" {
		return true
	}
	_, rest, service := serviceSplit(e.importee)
	return service && rest != "" && !containsSegment(rest, "internal") && !functionalProviderPorts[e.importee]
}

func functionalConfigurationFindings(file *ast.File, add addFinding) {
	ast.Inspect(file, func(node ast.Node) bool {
		field, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		name, ok := field.Key.(*ast.Ident)
		if ok && (name.Name == "Configure" || name.Name == "ConfigureEdges" || name.Name == "ConfigureRuntime") {
			add("functional-configuration", name.Name, "use customer CLI arguments and exact edges.Edges replacements", name.Pos())
		}
		return true
	})
}
