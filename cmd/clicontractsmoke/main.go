package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/generatedartifacts"
	rootapp "github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clicontract"
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
)

const successMessage = "[agent-factory:cli-contract-smoke] production CLI matches canonical and approved compatibility contracts"

func main() {
	root := flag.String("root", ".", "repository root")
	violation := flag.String("violation", "", "deliberate violation fixture")
	flag.Parse()
	store, err := generatedartifacts.NewLocalStore(platformfilesystem.Local{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[agent-factory:cli-contract-smoke] initialize source store: %v\n", err)
		os.Exit(1)
	}
	os.Exit(run(store, *root, *violation, os.Stdout, os.Stderr))
}

func run(store generatedartifacts.SourceStore, repositoryRoot, violation string, stdout, stderr io.Writer) int {
	if store == nil {
		fmt.Fprintln(stderr, "[agent-factory:cli-contract-smoke] source store is required")
		return 1
	}
	var observation cliobservation.Result
	process, err := rootapp.BuildProcess(context.Background(), serviceedges.Edges{
		CLIObserver: cliobservation.Capture(&observation),
	})
	if err != nil {
		fmt.Fprintf(stderr, "[agent-factory:cli-contract-smoke] build production process: %v\n", err)
		return 1
	}
	if err := process.Execute(rootapp.Input{
		Args: []string{"you"}, Env: os.Environ(), WorkingDirectory: repositoryRoot,
		Stdout: io.Discard, Stderr: io.Discard,
	}); err != nil {
		fmt.Fprintf(stderr, "[agent-factory:cli-contract-smoke] observe production command: %v\n", err)
		return 1
	}
	var (
		findings []clicontract.Finding
		checkErr error
	)
	if violation == "" {
		findings, checkErr = clicontract.CheckProduction(store, observation.Snapshot.Commands, repositoryRoot)
	} else {
		findings, checkErr = clicontract.CheckProductionViolation(store, observation.Snapshot.Commands, repositoryRoot, clicontract.DeliberateViolation(violation))
	}
	if checkErr != nil {
		fmt.Fprintf(stderr, "[agent-factory:cli-contract-smoke] check failed: %v\n", checkErr)
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
