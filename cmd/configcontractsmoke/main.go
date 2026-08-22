package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/configcontractsmoke"
	globalconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
)

const successMessage = "[agent-factory:config-contract-smoke] global, mock-worker, and Factory configuration contracts are aligned"

type checker func(string) ([]configcontractsmoke.Diagnostic, error)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()
	os.Exit(run(*root, os.Stdout, os.Stderr))
}

func run(root string, stdout, stderr io.Writer) int {
	return runWithChecker(root, stdout, stderr, func(repositoryRoot string) ([]configcontractsmoke.Diagnostic, error) {
		return configcontractsmoke.Check(repositoryRoot, func(payload []byte) error {
			_, err := globalconfig.Decode(payload)
			return err
		})
	})
}

func runWithChecker(root string, stdout, stderr io.Writer, check checker) int {
	diagnostics, err := check(root)
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:config-contract-smoke] check failed: %v\n", err)
		return 1
	}
	if len(diagnostics) > 0 {
		for _, diagnostic := range diagnostics {
			fmt.Fprintf(stderr, "[agent-factory:config-contract-smoke] %s family=%s (%s): %s\n", diagnostic.Path, diagnostic.Family, diagnostic.Code, diagnostic.Message)
		}
		fmt.Fprintln(stderr, "[agent-factory:config-contract-smoke] configuration contract parity failed; restore family, acceptance, export, staging, and ownership alignment")
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
