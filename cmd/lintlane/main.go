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
	makeTool        string
	checkerDriver   string
	checkerPackage  string
	checkerCacheDir string
	goTool          string
	reportFile      string
	jobs            int
	targets         []string
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

	runner := executeTarget
	cleanup := func() {}
	if cfg.checkerDriver != "" {
		runner = makeTargetRunner(cfg.checkerDriver)
	} else if cfg.checkerPackage != "" {
		checkerDriver, release, err := prepareCheckerDriver(cfg, stderr)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		cleanup = release
		runner = makeTargetRunner(checkerDriver)
	}
	defer cleanup()
	results := runTargets(cfg.makeTool, cfg.targets, cfg.jobs, runner)
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
	checkerDriver := flags.String("checker-driver", "", "compiled lint checker driver to pass to child Make targets")
	checkerPackage := flags.String("checker-package", "", "checker driver package to compile once for this lint lane")
	checkerCacheDir := flags.String("cache-dir", ".cache/lint-checkers", "directory for temporary lint lane executables")
	goTool := flags.String("go", "go", "Go executable used to compile the lint checker driver")
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
	if strings.TrimSpace(*checkerDriver) == "" && *checkerDriver != "" {
		return config{}, errors.New("checker driver must not be empty")
	}
	if strings.TrimSpace(*checkerPackage) == "" && *checkerPackage != "" {
		return config{}, errors.New("checker package must not be empty")
	}
	if strings.TrimSpace(*checkerCacheDir) == "" {
		return config{}, errors.New("checker cache directory must not be empty")
	}
	if strings.TrimSpace(*goTool) == "" {
		return config{}, errors.New("go tool must not be empty")
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
		makeTool:        *makeTool,
		checkerDriver:   *checkerDriver,
		checkerPackage:  *checkerPackage,
		checkerCacheDir: *checkerCacheDir,
		goTool:          *goTool,
		reportFile:      *reportFile,
		jobs:            jobs,
		targets:         targets,
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

func prepareCheckerDriver(cfg config, stderr io.Writer) (string, func(), error) {
	cacheDir, err := filepath.Abs(cfg.checkerCacheDir)
	if err != nil {
		return "", func() {}, fmt.Errorf("resolve lint lane cache directory: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", func() {}, fmt.Errorf("create lint lane cache directory: %w", err)
	}
	driverDir, err := os.MkdirTemp(cacheDir, ".lintlane-driver-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create lint lane driver directory: %w", err)
	}
	release := func() { _ = os.RemoveAll(driverDir) }
	driverPath := filepath.Join(driverDir, "lintcheck"+executableSuffix())
	command := execCommand(cfg.goTool, "build", "-o", driverPath, cfg.checkerPackage)
	command.Env = os.Environ()
	command.Stdout = io.Discard
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		release()
		return "", func() {}, fmt.Errorf("compile lint checker driver %s: %w", cfg.checkerPackage, err)
	}
	return driverPath, release, nil
}

func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func makeTargetRunner(checkerDriver string) targetRunner {
	return func(makeTool, target string, stdout, stderr io.Writer) error {
		return runMakeTargetWithVariables(makeTool, target, []string{"LINT_CHECKER_DRIVER=" + checkerDriver}, stdout, stderr)
	}
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
	return runMakeTargetWithVariables(makeTool, target, nil, stdout, stderr)
}

func runMakeTargetWithVariables(makeTool, target string, variables []string, stdout, stderr io.Writer) error {
	args := []string{"--no-print-directory"}
	args = append(args, variables...)
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
