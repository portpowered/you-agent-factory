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
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	var diagnostics []contractvalidator.Diagnostic
	for _, registry := range []contractvalidator.Registry{
		contractvalidator.CommonRegistry(),
		contractvalidator.CompatibilityInventoryRegistry(),
	} {
		diagnostics = append(diagnostics, contractvalidator.ValidateAll(root, registry)...)
	}
	return emitDiagnostics(diagnostics, stdout, stderr)
}

func runForRegistry(root string, registry contractvalidator.Registry, stdout, stderr io.Writer) int {
	return emitDiagnostics(contractvalidator.ValidateAll(root, registry), stdout, stderr)
}

func emitDiagnostics(diagnostics []contractvalidator.Diagnostic, stdout, stderr io.Writer) int {
	contractvalidator.SortDiagnostics(diagnostics)
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
