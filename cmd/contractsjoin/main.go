package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/contractjoiner"
)

const successMessage = "[agent-factory:contracts-join] joined contracts generated"

type stringList []string

func (values *stringList) String() string { return fmt.Sprint([]string(*values)) }
func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	input := contractjoiner.Input{}
	flag.StringVar(&input.RepositoryRoot, "repository-root", ".", "repository root")
	flag.Var((*stringList)(&input.Roots), "root", "repository-relative authored root (repeatable)")
	flag.Var((*stringList)(&input.Components), "component", "repository-relative authored component (repeatable)")
	flag.Parse()
	os.Exit(run(input, os.Stdout, os.Stderr))
}

func run(input contractjoiner.Input, stdout, stderr io.Writer) int {
	diagnostics := contractjoiner.Generate(input)
	if len(diagnostics) == 0 {
		fmt.Fprintln(stdout, successMessage)
		return 0
	}

	encoder := json.NewEncoder(stderr)
	encoder.SetEscapeHTML(false)
	for _, diagnostic := range diagnostics {
		if err := encoder.Encode(diagnostic); err != nil {
			fmt.Fprintln(stderr, "[agent-factory:contracts-join] diagnostic output failed")
			return 1
		}
	}
	return 1
}
