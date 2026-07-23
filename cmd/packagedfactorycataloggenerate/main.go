package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
)

const successMessage = "[agent-factory:packaged-factory-catalog-generate] generated complete catalog"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	if err := packagedfactorycatalog.Generate(root); err != nil {
		fmt.Fprintf(stderr, "[agent-factory:packaged-factory-catalog-generate] generation failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
