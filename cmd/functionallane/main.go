package main

import (
	"bytes"
	"flag"
	"fmt"
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

func main() {
	cfg := parseConfig()
	pkgs, err := discoverPackages(cfg.root)
	if err != nil {
		failf("discover functional packages: %v\n", err)
	}
	if len(pkgs) == 0 {
		failf("discover functional packages: no test packages found under %s\n", cfg.root)
	}
	if err := runFunctionalTests(cfg, pkgs); err != nil {
		failf("run functional lane: %v\n", err)
	}
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
	cmd := exec.Command("go", args...)
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

	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
