package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/portpowered/infinite-you/internal/releaseprep"
)

var (
	commandMain              = run
	exitFunc                 = os.Exit
	stdout         io.Writer = os.Stdout
	stderr         io.Writer = os.Stderr
	runReleasePrep           = releaseprep.Run
)

func main() {
	exitFunc(commandMain(os.Args[1:], stdout, stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	version, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	if err := runReleasePrep(context.Background(), releaseprep.Options{
		Version:        version,
		ProgressWriter: stdout,
	}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func parseArgs(args []string) (string, error) {
	var version string
	flags := flag.NewFlagSet("releaseprep", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&version, "version", "", "release semver tag, for example v1.2.3")
	if err := flags.Parse(args); err != nil {
		return "", err
	}
	return version, nil
}
