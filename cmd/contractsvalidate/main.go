package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/contractvalidator"
)

const successMessage = "[agent-factory:contracts-validate] registered contracts passed"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, contractvalidator.DefaultRegistry(), os.Stdout, os.Stderr))
}

func run(root string, registry contractvalidator.Registry, stdout, stderr io.Writer) int {
	diagnostics := contractvalidator.ValidateAll(root, registry)
	if len(diagnostics) == 0 {
		fmt.Fprintln(stdout, successMessage)
		return 0
	}

	encoder := json.NewEncoder(stderr)
	encoder.SetEscapeHTML(false)
	for _, diagnostic := range diagnostics {
		if err := encoder.Encode(diagnostic); err != nil {
			fmt.Fprintf(stderr, "[agent-factory:contracts-validate] write diagnostic: %v\n", err)
			return 1
		}
	}
	return 1
}
