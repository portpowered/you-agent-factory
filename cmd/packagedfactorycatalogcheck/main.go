package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
)

const successMessage = "[agent-factory:packaged-factory-catalog-check] generated catalog is current"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	drift, err := packagedfactorycatalog.Check(root)
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:packaged-factory-catalog-check] check failed: %v\n", err)
		return 1
	}
	if drift.Empty() {
		fmt.Fprintln(stdout, successMessage)
		return 0
	}
	writePaths(stderr, "stale", drift.Stale)
	writePaths(stderr, "missing", drift.Missing)
	writePaths(stderr, "unexpected", drift.Unexpected)
	fmt.Fprintln(
		stderr,
		"[agent-factory:packaged-factory-catalog-check] generated catalog differs from canonical authored inventory; run `make packaged-factory-catalog-generate` and remove every unexpected generated output",
	)
	return 1
}

func writePaths(writer io.Writer, category string, paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Fprintf(writer, "[agent-factory:packaged-factory-catalog-check] %s:\n", category)
	for _, path := range paths {
		fmt.Fprintf(writer, "  %s\n", path)
	}
}
