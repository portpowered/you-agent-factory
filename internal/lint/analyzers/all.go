package analyzers

import "golang.org/x/tools/go/analysis"

// All returns every repository analyzer in the order they are registered in
// the cmd/repolint multichecker.
func All() []*analysis.Analyzer {
	return []*analysis.Analyzer{Layering, Behavior, Construction, Petripublic, DurableConstruction, RegisteredConstruction}
}

func init() {
	for _, analyzer := range All() {
		analyzer.Flags.Bool("check-stale", true, "reject stale exact debt entries (disable only with a mandatory complete-tag run)")
	}
}
