package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type config struct {
	count   int
	jobs    int
	root    string
	short   bool
	timeout time.Duration
}

type packageResult struct {
	name     string
	duration time.Duration
	output   string
	err      error
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

	results, err := runPackages(cfg, pkgs)
	printResults(results)
	if err != nil {
		failf("%v\n", err)
	}
}

func parseConfig() config {
	var cfg config
	flag.IntVar(&cfg.count, "count", 1, "go test -count value")
	flag.IntVar(&cfg.jobs, "jobs", defaultJobs(), "number of package test subprocesses to run at once")
	flag.StringVar(&cfg.root, "root", filepath.ToSlash(filepath.Join("tests", "functional")), "functional test root")
	flag.BoolVar(&cfg.short, "short", true, "run with go test -short")
	flag.DurationVar(&cfg.timeout, "timeout", 5*time.Minute, "per-package go test timeout")
	flag.Parse()
	if cfg.jobs < 1 {
		cfg.jobs = 1
	}
	return cfg
}

func defaultJobs() int {
	switch n := runtime.NumCPU(); {
	case n >= 8:
		return 2
	case n >= 4:
		return 2
	default:
		return 1
	}
}

func discoverPackages(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var pkgs []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "internal" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, entry.Name(), "*_test.go"))
		if err != nil {
			return nil, err
		}
		if len(matches) == 0 {
			continue
		}
		pkgs = append(pkgs, "./"+filepath.ToSlash(filepath.Join(root, entry.Name())))
	}
	sort.Strings(pkgs)
	return pkgs, nil
}

func runPackages(cfg config, pkgs []string) ([]packageResult, error) {
	results := make([]packageResult, len(pkgs))
	workCh := make(chan int)
	errCh := make(chan error, len(pkgs))
	var wg sync.WaitGroup

	for range cfg.jobs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range workCh {
				results[idx] = runPackage(cfg, pkgs[idx])
				if results[idx].err != nil {
					errCh <- fmt.Errorf("%s failed", pkgs[idx])
				}
			}
		}()
	}

	for idx := range pkgs {
		workCh <- idx
	}
	close(workCh)
	wg.Wait()
	close(errCh)

	if len(errCh) == 0 {
		return results, nil
	}
	return results, fmt.Errorf("functional lane failed")
}

func runPackage(cfg config, pkg string) packageResult {
	args := []string{"test"}
	if cfg.short {
		args = append(args, "-short")
	}
	args = append(args,
		pkg,
		fmt.Sprintf("-count=%d", cfg.count),
		fmt.Sprintf("-timeout=%s", cfg.timeout),
	)

	cmd := exec.Command("go", args...)
	cmd.Env = os.Environ()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	start := time.Now()
	err := cmd.Run()
	return packageResult{
		name:     pkg,
		duration: time.Since(start),
		output:   output.String(),
		err:      err,
	}
}

func printResults(results []packageResult) {
	for _, result := range results {
		if result.output != "" {
			fmt.Print(strings.TrimRight(result.output, "\n"))
			fmt.Println()
		}
		fmt.Printf("[functionallane] %s finished in %s\n", result.name, result.duration.Round(10*time.Millisecond))
	}
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
