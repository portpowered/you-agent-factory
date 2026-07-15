package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clicontract"
)

const successMessage = "[agent-factory:cli-contract-smoke] production CLI matches canonical and approved compatibility contracts"

func main() {
	root := flag.String("root", ".", "repository root")
	violation := flag.String("violation", "", "deliberate violation fixture")
	flag.Parse()
	os.Exit(run(*root, *violation, os.Stdout, os.Stderr))
}

func run(repositoryRoot, violation string, stdout, stderr io.Writer) int {
	root := cli.NewRootCommand()
	var (
		findings []clicontract.Finding
		err      error
	)
	if violation == "" {
		findings, err = clicontract.CheckProduction(root, repositoryRoot)
	} else {
		findings, err = clicontract.CheckProductionViolation(root, repositoryRoot, clicontract.DeliberateViolation(violation))
	}
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:cli-contract-smoke] check failed: %v\n", err)
		return 1
	}
	if len(findings) > 0 {
		for _, finding := range findings {
			fmt.Fprintf(stderr, "[agent-factory:cli-contract-smoke] %s\n", finding.Error())
		}
		return 1
	}
	fmt.Fprintln(stdout, successMessage)
	return 0
}
