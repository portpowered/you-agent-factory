package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/mcpcontractcheck"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
)

const (
	commandPrefix  = "[agent-factory:mcp-tool-inventory-generate]"
	successMessage = commandPrefix + " MCP tool inventory snapshot generated"
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	store, err := generatedartifacts.NewLocalStore(platformfilesystem.Local{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s initialize artifact store: %v\n", commandPrefix, err)
		os.Exit(1)
	}
	os.Exit(run(store, *root, os.Stdout, os.Stderr))
}

func run(store generatedartifacts.Store, root string, stdout, stderr io.Writer) int {
	if store == nil {
		fmt.Fprintf(stderr, "%s artifact store is required\n", commandPrefix)
		return 1
	}

	artifacts, err := mcpcontractcheck.GenerateInventoryArtifacts()
	if err != nil {
		fmt.Fprintf(stderr, "%s generation failed: %v\n", commandPrefix, err)
		return 1
	}
	if err := store.Write(root, artifacts); err != nil {
		fmt.Fprintf(stderr, "%s generation failed: %v\n", commandPrefix, err)
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
