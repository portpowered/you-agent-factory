package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/portpowered/infinite-you/internal/releasesmoke"
)

var (
	commandMain               = run
	exitFunc                  = os.Exit
	stdout          io.Writer = os.Stdout
	stderr          io.Writer = os.Stderr
	runReleaseSmoke           = releasesmoke.Run
)

func main() {
	exitFunc(commandMain(os.Args[1:], stdout, stderr))
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	cfg, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	result, err := runReleaseSmoke(context.Background(), cfg)
	if err != nil {
		writeJSON(stderr, err)
		return 1
	}
	writeJSON(stdout, result)
	return 0
}

func parseArgs(args []string) (releasesmoke.Config, error) {
	var cfg releasesmoke.Config
	flags := flag.NewFlagSet("releasesmoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.BinaryPath, "binary", "", "path to the extracted agent-factory binary to smoke test")
	flags.StringVar(&cfg.FixturePath, "fixture", "", "path to the canonical release smoke fixture directory")
	flags.DurationVar(&cfg.Timeout, "timeout", 60*time.Second, "overall smoke timeout")
	if err := flags.Parse(args); err != nil {
		return releasesmoke.Config{}, err
	}
	return cfg, nil
}

func writeJSON(writer io.Writer, value any) {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintf(stderr, "encode JSON output: %v\n", err)
	}
}
