package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

const supportPackageSuffix = "/internal/support"

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
	pkgs, err := discoverPackages(cfg.root)
	if err != nil {
		return fmt.Errorf("discover functional packages: %w", err)
	}
	if len(pkgs) == 0 {
		return fmt.Errorf("discover functional packages: no test packages found under %s", cfg.root)
	}

	if err := runFunctionalTests(cfg, pkgs); err != nil {
		return fmt.Errorf("run functional lane: %w", err)
	}

	return nil
}

func parseConfig() config {
	var cfg config
	flag.IntVar(&cfg.count, "count", 1, "go test -count value")
	flag.IntVar(&cfg.jobs, "jobs", 2, "go test -p value")
	flag.StringVar(&cfg.root, "root", "./tests/functional/...", "go list package pattern for functional test discovery")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func discoverPackages(root string) ([]string, error) {
	args := []string{"list", root}
	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w\n%s", err, strings.TrimSpace(stderr.String()))
	}

	lines := strings.Split(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n")
	pkgs := make([]string, 0, len(lines))
	for _, line := range lines {
		pkg := strings.TrimSpace(line)
		if pkg == "" || strings.HasSuffix(pkg, supportPackageSuffix) {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

func runFunctionalTests(cfg config, pkgs []string) error {
	args := []string{"test", fmt.Sprintf("-p=%d", cfg.jobs)}
	if cfg.short {
		args = append(args, "-short")
	}
	args = append(args, pkgs...)
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
