package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	factorysession "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/mcp"
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

	payload, err := factorysession.GenerateToolInventoryJSON()
	if err != nil {
		fmt.Fprintf(stderr, "%s generation failed: %v\n", commandPrefix, err)
		return 1
	}
	artifact := generatedartifacts.Artifact{
		Path:    factorysession.ToolInventoryBaselineRelativePath,
		Payload: payload,
	}
	if err := store.Write(root, []generatedartifacts.Artifact{artifact}); err != nil {
		fmt.Fprintf(stderr, "%s generation failed: %v\n", commandPrefix, err)
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
