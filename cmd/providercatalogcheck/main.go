package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/providercatalog"
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	drift, err := providercatalog.Check(root)
	if err != nil {
		fmt.Fprintf(stderr, "[provider-catalog-check] check failed: %v\n", err)
		return 1
	}
	if drift.Empty() {
		fmt.Fprintln(stdout, "[provider-catalog-check] generated provider catalog is current")
		return 0
	}
	reportDrift(stderr, drift)
	return 1
}

func reportDrift(stderr io.Writer, drift providercatalog.Drift) {
	writeDrift(stderr, "stale", drift.Stale)
	writeDrift(stderr, "missing", drift.Missing)
	writeDrift(stderr, "unexpected", drift.Unexpected)
	fmt.Fprintln(stderr, "[provider-catalog-check] generated provider catalog differs from authored inputs; run `make provider-catalog-generate`")
}

func writeDrift(writer io.Writer, category string, paths []string) {
	for _, path := range paths {
		fmt.Fprintf(writer, "[provider-catalog-check] %s: %s\n", category, path)
	}
}
