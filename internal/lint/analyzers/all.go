package analyzers

import "golang.org/x/tools/go/analysis"

// All returns every repository analyzer in the order they are registered in
// the cmd/repolint multichecker.
func All() []*analysis.Analyzer {
	return []*analysis.Analyzer{Layering, Behavior, DurableConstruction, RegisteredConstruction}
}
