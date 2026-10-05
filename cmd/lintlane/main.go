package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultLintJobs = 4

type config struct {
	makeTool   string
	reportFile string
	jobs       int
	targets    []string
}

type targetResult struct {
	target   string
	output   string
	err      error
	duration time.Duration
}

type targetRunner func(makeTool, target string, stdout, stderr io.Writer) error

type targetWork struct {
	index  int
	target string
}

var (
	executeTarget = runMakeTarget
	execCommand   = exec.Command
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	started := time.Now()
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}

	results := runTargets(cfg.makeTool, cfg.targets, cfg.jobs, executeTarget)
	if cfg.reportFile != "" {
		if err := writeReportFile(cfg.reportFile, cfg.jobs, time.Since(started), results); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if err := writeReport(stdout, results); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("lintlane", flag.ContinueOnError)
	flags.SetOutput(stderr)
	makeTool := flags.String("make", "make", "Make executable used to run each lint target")
	reportFile := flags.String("report-file", "", "JSON file for the complete per-target lint report")
	jobsRaw := flags.String("jobs", strconv.Itoa(defaultLintJobs), "maximum number of lint targets to run concurrently")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	jobs, err := parseLintJobs(*jobsRaw)
	if err != nil {
		return config{}, err
	}
	if strings.TrimSpace(*makeTool) == "" {
		return config{}, errors.New("make executable must not be empty")
	}
	if strings.TrimSpace(*reportFile) == "" && *reportFile != "" {
		return config{}, errors.New("report file must not be empty")
	}
	targets := append([]string(nil), flags.Args()...)
	if len(targets) == 0 {
		return config{}, errors.New("at least one lint target is required")
	}
	for _, target := range targets {
		if strings.TrimSpace(target) == "" {
			return config{}, errors.New("lint target names must not be empty")
		}
	}
	return config{
		makeTool:   *makeTool,
		reportFile: *reportFile,
		jobs:       jobs,
		targets:    targets,
	}, nil
}

func parseLintJobs(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("invalid -jobs value: expected a positive integer, received %q", raw)
	}
	jobs, err := strconv.Atoi(raw)
	if err != nil || jobs < 1 {
		return 0, fmt.Errorf("invalid -jobs value: expected a positive integer, received %q", raw)
	}
	return jobs, nil
}

func runTargets(makeTool string, targets []string, jobs int, runner targetRunner) []targetResult {
	if len(targets) == 0 {
		return nil
	}
	workerCount := jobs
	if workerCount > len(targets) {
		workerCount = len(targets)
	}

	work := make(chan targetWork)
	results := make([]targetResult, len(targets))
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for item := range work {
				started := time.Now()
				output := &lockedBuffer{}
				err := runner(makeTool, item.target, output, output)
				results[item.index] = targetResult{
					target:   item.target,
					output:   output.String(),
					err:      err,
					duration: time.Since(started),
				}
			}
		}()
	}
	for index, target := range targets {
		work <- targetWork{index: index, target: target}
	}
	close(work)
	workers.Wait()
	return results
}

func runMakeTarget(makeTool, target string, stdout, stderr io.Writer) error {
	args := []string{"--no-print-directory"}
	if shell := windowsMakeShell(); shell != "" {
		args = append(args, "SHELL="+shell)
	}
	args = append(args, target)
	command := execCommand(makeTool, args...)
	temporaryDirectory, err := makeTargetTemporaryDirectory()
	if err != nil {
		return err
	}
	if temporaryDirectory != "" {
		defer os.RemoveAll(temporaryDirectory)
	}
	command.Env = makeTargetEnvironment(temporaryDirectory)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	return command.Run()
}

func windowsMakeShell() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	if shell := os.Getenv("SHELL"); shell != "" && isRegularFile(shell) {
		return shell
	}
	if shell, err := exec.LookPath("sh.exe"); err == nil {
		return shell
	}
	gitTool, err := exec.LookPath("git.exe")
	if err == nil {
		gitDirectory := filepath.Dir(gitTool)
		for _, relativePath := range []string{
			filepath.Join("..", "bin", "sh.exe"),
			filepath.Join("..", "usr", "bin", "sh.exe"),
		} {
			shell := filepath.Clean(filepath.Join(gitDirectory, relativePath))
			if isRegularFile(shell) {
				return shell
			}
		}
	}
	return ""
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func makeTargetTemporaryDirectory() (string, error) {
	if runtime.GOOS != "windows" {
		return "", nil
	}
	temporaryDirectory, err := os.MkdirTemp("", "lintlane-target-")
	if err != nil {
		return "", fmt.Errorf("create lint target temporary directory: %w", err)
	}
	return temporaryDirectory, nil
}

func makeTargetEnvironment(temporaryDirectory string) []string {
	if temporaryDirectory == "" {
		return os.Environ()
	}
	environment := os.Environ()
	for _, key := range []string{"TEMP", "TMP", "TMPDIR"} {
		environment = withEnvironment(environment, key, temporaryDirectory)
	}
	return environment
}

func withEnvironment(environment []string, key, value string) []string {
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || name != key {
			filtered = append(filtered, entry)
		}
	}
	return append(filtered, key+"="+value)
}

func writeReport(stdout io.Writer, results []targetResult) error {
	failed := make([]string, 0)
	for _, result := range results {
		if err := writeTargetReport(stdout, result); err != nil {
			return err
		}
		if result.err != nil {
			failed = append(failed, result.target)
		}
	}
	if len(failed) == 0 {
		_, err := fmt.Fprintf(stdout, "LINT PASSED: %d target(s) completed successfully\n", len(results))
		return err
	}
	if err := writeFailureSummary(stdout, failed); err != nil {
		return err
	}
	return fmt.Errorf("lint failed for target(s): %s", strings.Join(failed, ", "))
}

func writeTargetReport(stdout io.Writer, result targetResult) error {
	if _, err := fmt.Fprintf(stdout, "===== lint target: %s =====\n", result.target); err != nil {
		return err
	}
	if result.output != "" {
		if _, err := io.WriteString(stdout, result.output); err != nil {
			return err
		}
		if !strings.HasSuffix(result.output, "\n") {
			if _, err := io.WriteString(stdout, "\n"); err != nil {
				return err
			}
		}
	}
	status := "PASS"
	if result.err != nil {
		status = "FAIL"
		if _, err := fmt.Fprintf(stdout, "command error: %v\n", result.err); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(stdout, "===== lint target: %s: %s =====\n", result.target, status)
	return err
}

func writeFailureSummary(stdout io.Writer, failed []string) error {
	if _, err := fmt.Fprintf(stdout, "LINT FAILED: %d target(s)\n", len(failed)); err != nil {
		return err
	}
	for _, target := range failed {
		if _, err := fmt.Fprintf(stdout, "  %s (rerun: make %s)\n", target, target); err != nil {
			return err
		}
	}
	return nil
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
