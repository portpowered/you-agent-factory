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
	if err := providercatalog.Generate(root); err != nil {
		fmt.Fprintf(stderr, "[provider-catalog-generate] generation failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, "[provider-catalog-generate] generated provider catalog and schemas")
	return 0
}
