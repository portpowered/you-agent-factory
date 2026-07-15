package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/javascriptcontractsmoke"
)

const successMessage = "[agent-factory:javascript-contract-smoke] catalog, projection, binding descriptor, and behavior baseline are aligned"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	diagnostics, err := javascriptcontractsmoke.Check(root)
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:javascript-contract-smoke] check failed: %v\n", err)
		return 1
	}
	if len(diagnostics) > 0 {
		for _, diagnostic := range diagnostics {
			fmt.Fprintf(
				stderr,
				"[agent-factory:javascript-contract-smoke] %s (%s): %s\n",
				diagnostic.Path,
				diagnostic.Code,
				diagnostic.Message,
			)
		}
		fmt.Fprintln(stderr, "[agent-factory:javascript-contract-smoke] JavaScript contract parity failed; restore catalog, staging, binding descriptor, and call-behavior baseline alignment")
		return 1
	}

	fmt.Fprintln(stdout, successMessage)
	return 0
}
