package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type config struct {
	baseRef  string
	headRef  string
	repoRoot string
	attempts int
	budget   time.Duration
	jobs     int
}

var (
	stdoutWriter io.Writer = os.Stdout
	stderrWriter io.Writer = os.Stderr
	exitFunc               = os.Exit
)

func main() {
	cfg := parseConfig()
	if err := run(cfg); err != nil {
		fmt.Fprintln(stderrWriter, err)
		exitFunc(1)
	}
}

func parseConfig() config {
	cfg := config{}
	flag.StringVar(&cfg.baseRef, "base", "", "pull-request base ref or SHA; its merge-base with head is used")
	flag.StringVar(&cfg.headRef, "head", "", "pull-request head ref or SHA")
	flag.StringVar(&cfg.repoRoot, "repo-root", ".", "repository working directory")
	flag.IntVar(&cfg.attempts, "attempts", defaultStabilityAttempts, "isolated attempts per selected test")
	flag.DurationVar(&cfg.budget, "budget", defaultStabilityBudget, "total stability execution budget")
	flag.IntVar(&cfg.jobs, "jobs", defaultStabilityJobs, "maximum concurrent isolated test attempts")
	flag.Parse()
	return cfg
}

func run(cfg config) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	repoRoot, err := resolveRepositoryRoot(cfg.repoRoot)
	if err != nil {
		return err
	}
	mergeBase, err := gitMergeBase(repoRoot, cfg.baseRef, cfg.headRef)
	if err != nil {
		return err
	}
	rawDiff, err := gitDiff(repoRoot, mergeBase, cfg.headRef)
	if err != nil {
		return err
	}
	selected, err := selectChangedTests(rawDiff, cfg.headRef, mergeBase, func(revision, path string) ([]byte, error) {
		return gitShow(repoRoot, revision, path)
	})
	if err != nil {
		return err
	}
	groups, err := groupSelectedTests(selected, func(path string) (string, error) {
		return goPackageForFile(repoRoot, path)
	})
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdoutWriter, "Changed-test stability: base=%s merge-base=%s head=%s\n", cfg.baseRef, mergeBase, cfg.headRef)
	runner := stabilityRunner{
		attempts: cfg.attempts,
		budget:   cfg.budget,
		jobs:     cfg.jobs,
		run: func(ctx context.Context, group testGroup, testName string) (string, error) {
			return runGoTestAttempt(ctx, repoRoot, group, testName)
		},
		runGroup: func(ctx context.Context, group testGroup) (string, error) {
			return runGoTestGroupAttempt(ctx, repoRoot, group)
		},
	}
	return runner.runAll(groups, stdoutWriter)
}

func validateConfig(cfg config) error {
	if strings.TrimSpace(cfg.baseRef) == "" || strings.TrimSpace(cfg.headRef) == "" {
		return errors.New("changed-test stability requires both -base and -head")
	}
	if cfg.attempts < 1 {
		return errors.New("changed-test stability -attempts must be greater than zero")
	}
	if cfg.budget <= 0 {
		return errors.New("changed-test stability -budget must be greater than zero")
	}
	if cfg.jobs < 1 {
		return errors.New("changed-test stability -jobs must be greater than zero")
	}
	return nil
}

func resolveRepositoryRoot(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve repository root %q: %w", path, err)
	}
	command := exec.Command("git", "-C", absolute, "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve repository root %q: %w", path, err)
	}
	return strings.TrimSpace(string(output)), nil
}

func gitMergeBase(repoRoot, baseRef, headRef string) (string, error) {
	command := exec.Command("git", "-C", repoRoot, "merge-base", baseRef, headRef)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("resolve merge base for %s and %s: %w", baseRef, headRef, err)
	}
	mergeBase := strings.TrimSpace(string(output))
	if mergeBase == "" {
		return "", fmt.Errorf("resolve merge base for %s and %s: empty result", baseRef, headRef)
	}
	return mergeBase, nil
}

func gitDiff(repoRoot, baseRevision, headRevision string) (string, error) {
	command := exec.Command("git", "-C", repoRoot, "diff", "--find-renames", "--unified=0", "--no-ext-diff", baseRevision, headRevision, "--")
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("read merge-base test diff %s..%s: %w", baseRevision, headRevision, err)
	}
	return string(output), nil
}

func gitShow(repoRoot, revision, path string) ([]byte, error) {
	command := exec.Command("git", "-C", repoRoot, "show", revision+":"+filepath.ToSlash(path))
	output, err := command.Output()
	if err != nil {
		return nil, err
	}
	return output, nil
}

func goPackageForFile(repoRoot, path string) (string, error) {
	directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(path)))
	pattern := "."
	if directory != "." {
		pattern = "./" + strings.TrimPrefix(directory, "./")
	}
	command := exec.Command("go", "list", "-f={{.ImportPath}}", pattern)
	command.Dir = repoRoot
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("go list %s: %w", pattern, err)
	}
	packagePath := strings.TrimSpace(string(output))
	if strings.Contains(packagePath, "\n") {
		return "", fmt.Errorf("go list %s returned multiple packages", pattern)
	}
	return packagePath, nil
}
