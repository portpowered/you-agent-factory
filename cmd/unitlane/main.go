package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"
)

const modulePath = "github.com/portpowered/infinite-you"

var excludedSuiteRoots = []string{
	modulePath + "/tests/functional",
	modulePath + "/tests/stress",
	modulePath + "/tests/release",
}

type config struct {
	count   int
	jobs    int
	root    string
	short   bool
	timeout time.Duration
}

var executeUnitLane = run
var execCommand = exec.Command
var stderrWriter io.Writer = os.Stderr
var exitFunc = os.Exit

func main() {
	if err := executeUnitLane(); err != nil {
		fmt.Fprintf(stderrWriter, "%v\n", err)
		exitFunc(1)
	}
}

func run() error {
	cfg := parseConfig()
	packages, err := discoverPackages(cfg.root)
	if err != nil {
		return fmt.Errorf("discover unit packages: %w", err)
	}
	if len(packages) == 0 {
		return fmt.Errorf("discover unit packages: no packages found under %s", cfg.root)
	}
	if err := runUnitTests(cfg, packages); err != nil {
		return fmt.Errorf("run unit lane: %w", err)
	}
	return nil
}

func parseConfig() config {
	var cfg config
	flag.IntVar(&cfg.count, "count", 1, "go test -count value")
	flag.IntVar(&cfg.jobs, "jobs", 2, "go test -p value")
	flag.StringVar(&cfg.root, "root", "./...", "go list package pattern for unit test discovery")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "go test timeout")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func discoverPackages(root string) ([]string, error) {
	cmd := execCommand("go", "list", root)
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, commandError(err, stderr.String())
	}

	packages := make([]string, 0)
	for _, line := range strings.Split(strings.ReplaceAll(stdout.String(), "\r\n", "\n"), "\n") {
		pkg := strings.TrimSpace(line)
		if pkg != "" && isUnitPackage(pkg) {
			packages = append(packages, pkg)
		}
	}
	slices.Sort(packages)
	return slices.Compact(packages), nil
}

func isUnitPackage(importPath string) bool {
	for _, root := range excludedSuiteRoots {
		if importPath == root || strings.HasPrefix(importPath, root+"/") {
			return false
		}
	}
	return true
}

func runUnitTests(cfg config, packages []string) error {
	args := []string{"test", fmt.Sprintf("-p=%d", cfg.jobs)}
	if cfg.short {
		args = append(args, "-short")
	}
	args = append(args, packages...)
	args = append(args, fmt.Sprintf("-count=%d", cfg.count), fmt.Sprintf("-timeout=%s", cfg.timeout))

	cmd := execCommand("go", args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandError(err error, stderr string) error {
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		return err
	}
	return fmt.Errorf("%w\n%s", err, detail)
}
