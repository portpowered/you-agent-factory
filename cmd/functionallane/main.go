package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

type config struct {
	count   int
	jobs    int
	root    string
	short   bool
	timeout time.Duration
}

var executeFunctionalLane = run
var execCommand = exec.Command
var stderrWriter io.Writer = os.Stderr
var exitFunc = os.Exit

func main() {
	if err := executeFunctionalLane(); err != nil {
		failf("%v\n", err)
	}
}

func run() error {
	cfg := parseConfig()
	if err := runFunctionalTests(cfg); err != nil {
		return fmt.Errorf("run functional lane: %w", err)
	}

	return nil
}

func parseConfig() config {
	var cfg config
	flag.IntVar(&cfg.count, "count", 1, "go test -count value")
	flag.IntVar(&cfg.jobs, "jobs", 8, "go test -p value")
	flag.StringVar(&cfg.root, "root", "./tests/functional/...", "go test package pattern for the functional lane")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func runFunctionalTests(cfg config) error {
	args := []string{"test", fmt.Sprintf("-p=%d", cfg.jobs)}
	if cfg.short {
		args = append(args, "-short")
	}
	args = append(args, cfg.root)
	args = append(args,
		fmt.Sprintf("-count=%d", cfg.count),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)

	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func failf(format string, args ...any) {
	fmt.Fprintf(stderrWriter, format, args...)
	exitFunc(1)
}
