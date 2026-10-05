// Command repolint is the single multichecker binary that carries every
// repository-specific go/analysis analyzer. Run it through go vet so it rides
// the Go build graph and cache:
//
//	go build -o bin/repolint ./cmd/repolint
//	go vet -vettool=bin/repolint ./...
package main

import (
	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/portpowered/infinite-you/internal/lint/analyzers"
)

func main() {
	multichecker.Main(analyzers.All()...)
}
