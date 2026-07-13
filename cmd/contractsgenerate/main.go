package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
	"github.com/portpowered/infinite-you/internal/contractstaging"
)

const successMessage = "[agent-factory:contracts-generate] approved joined contracts generated"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	diagnostics := contractjoiner.Generate(contractstaging.JoinInput(root))
	if len(diagnostics) == 0 {
		fmt.Fprintln(stdout, successMessage)
		return 0
	}

	encoder := json.NewEncoder(stderr)
	encoder.SetEscapeHTML(false)
	for _, diagnostic := range diagnostics {
		if err := encoder.Encode(diagnostic); err != nil {
			fmt.Fprintln(stderr, "[agent-factory:contracts-generate] diagnostic output failed")
			return 1
		}
	}
	return 1
}
