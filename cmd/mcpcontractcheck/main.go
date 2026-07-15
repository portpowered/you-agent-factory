package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
)

const successMessage = "[agent-factory:mcp-contract-check] authored catalog, generated discovery, and handwritten handler registry are aligned"

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	diagnostics, err := mcpcontractcheck.Check(root)
	return report(diagnostics, err, stdout, stderr)
}

func report(diagnostics []mcpcontractcheck.Diagnostic, err error, stdout, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:mcp-contract-check] check failed: %v\n", err)
		return 1
	}
	if len(diagnostics) != 0 {
		for _, diagnostic := range diagnostics {
			fmt.Fprintf(stderr, "[agent-factory:mcp-contract-check] %s (%s, %s): %s\n", diagnostic.ToolID, diagnostic.Surface, diagnostic.Code, diagnostic.Message)
		}
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
