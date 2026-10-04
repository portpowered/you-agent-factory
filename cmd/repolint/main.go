// Command repolint is the single multichecker binary that carries every
// repository-specific go/analysis analyzer. Run it through go vet so it rides
// the Go build graph and cache:
//
//	go build -o bin/repolint ./cmd/repolint
//	go vet -vettool=bin/repolint ./...
//
// Compare the embedded exact debt against one merge-base baseline object:
//
//	repolint -baseline-growth=<base-file>
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis/multichecker"

	"github.com/portpowered/infinite-you/internal/lint/analyzers"
)

func main() {
	if handled, err := baselineMode(os.Args[1:], os.Stdout); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	multichecker.Main(analyzers.All()...)
}

func baselineMode(args []string, out io.Writer) (bool, error) {
	for _, arg := range args {
		if arg == "-baseline-growth" || strings.HasPrefix(arg, "-baseline-growth=") {
			if len(args) != 1 || !strings.HasPrefix(arg, "-baseline-growth=") || strings.TrimPrefix(arg, "-baseline-growth=") == "" {
				return true, fmt.Errorf("usage: repolint -baseline-growth=<base-file>")
			}
			data, err := os.ReadFile(strings.TrimPrefix(arg, "-baseline-growth="))
			if err != nil {
				return true, fmt.Errorf("read merge-base baseline: %w", err)
			}
			seeded, err := analyzers.CheckBaselineGrowth(string(data))
			if err != nil {
				return true, err
			}
			_, err = fmt.Fprintf(out, "lint-baseline-growth: accepted; seeded rule IDs: %s\n", strings.Join(seeded, ", "))
			return true, err
		}
	}
	return false, nil
}
