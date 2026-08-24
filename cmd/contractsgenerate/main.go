package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/javascriptcontract"
)

const successMessage = "[agent-factory:contracts-generate] approved contract artifacts generated"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	if err := javascriptcontract.GenerateJavaScriptWorkflowReference(root); err != nil {
		fmt.Fprintf(stderr, "[agent-factory:contracts-generate] generation failed: %v\n", err)
		return 1
	}
	if err := contractstaging.Generate(root); err != nil {
		fmt.Fprintf(stderr, "[agent-factory:contracts-generate] generation failed: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
