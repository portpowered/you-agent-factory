package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type config struct {
	goBinary                string
	repositoryRoot          string
	minimumCoverage         float64
	packageManifest         string
	packageFloorPolicy      string
	testTimeout             string
	profilePath             string
	coveragePath            string
	timingPath              string
	logPath                 string
	jobs                    int
	jobsSet                 bool
	coverageDiagnosticsPath string
}

type packageCoverage struct {
	Package         string  `json:"package"`
	CoveragePercent float64 `json:"coveragePercent"`
}

type coverageSummary struct {
	Packages []packageCoverage `json:"packages"`
}

type packageTiming struct {
	Package string  `json:"package"`
	Seconds float64 `json:"seconds"`
}

type timingSummary struct {
	Packages []packageTiming `json:"packages"`
}

type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string { return e.err.Error() }
func (e exitError) Unwrap() error { return e.err }

func main() {
	cfg := parseConfig()
	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var status exitError
		if errors.As(err, &status) {
			os.Exit(status.code)
		}
		os.Exit(1)
	}
}

func parseConfig() config {
	cfg, err := parseConfigArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	return cfg
}

func parseConfigArgs(args []string) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("unitcoverage", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.goBinary, "go", "go", "Go executable used to run gocoveragecheck")
	flags.StringVar(&cfg.repositoryRoot, "root", ".", "repository root")
	flags.Float64Var(&cfg.minimumCoverage, "minimum", 75.9, "minimum aggregate unit coverage")
	flags.StringVar(&cfg.packageManifest, "package-manifest", "docs/internal/baselines/go-unit-coverage-package-minimums.json", "unit package-floor manifest")
	flags.StringVar(&cfg.packageFloorPolicy, "package-floor-policy", "blocking", "unit package-floor policy")
	flags.StringVar(&cfg.testTimeout, "test-timeout", "10m", "gocoveragecheck test timeout")
	flags.StringVar(&cfg.profilePath, "profile", "", "coverage profile path")
	flags.StringVar(&cfg.coveragePath, "coverage-summary", "", "coverage summary JSON path")
	flags.StringVar(&cfg.timingPath, "timing-summary", "", "unit timing summary JSON path")
	flags.StringVar(&cfg.logPath, "log", "", "complete command log path")
	flags.IntVar(&cfg.jobs, "jobs", 0, "maximum concurrent unit coverage packages; omitted keeps gocoveragecheck's platform default")
	flags.StringVar(&cfg.coverageDiagnosticsPath, "coverage-build-diagnostics", "", "optional coverage build diagnostic JSON path")
	if err := flags.Parse(args); err != nil {
		return config{}, fmt.Errorf("parse unit coverage options: %w", err)
	}
	flags.Visit(func(flag *flag.Flag) {
		if flag.Name == "jobs" {
			cfg.jobsSet = true
		}
	})
	if len(flags.Args()) > 0 {
		return config{}, fmt.Errorf("parse unit coverage options: unexpected arguments %q", flags.Args())
	}
	return cfg, nil
}

func run(cfg config, stdout io.Writer) error {
	if err := validateConfig(cfg); err != nil {
		return err
	}
	for _, path := range ownedArtifactPaths(cfg) {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale unit coverage artifact %s: %w", path, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(cfg.logPath), 0o755); err != nil {
		return fmt.Errorf("create unit coverage artifact directory: %w", err)
	}
	logFile, err := os.OpenFile(cfg.logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open unit coverage command log: %w", err)
	}
	defer logFile.Close()

	args := coverageCommandArgs(cfg)
	commandErr := commandRunner(commandInvocation{
		name:         cfg.goBinary,
		args:         args,
		dir:          cfg.repositoryRoot,
		stdoutWriter: logFile,
		stderrWriter: logFile,
	})

	if err := renderConsoleSummary(cfg.coveragePath, cfg.timingPath, stdout); err != nil {
		_, _ = fmt.Fprintf(logFile, "Unit coverage console summary unavailable: %v\n", err)
		if commandErr == nil {
			return err
		}
	}
	if commandErr == nil {
		return nil
	}
	var status exitError
	if errors.As(commandErr, &status) {
		return status
	}
	var commandExit *exec.ExitError
	if errors.As(commandErr, &commandExit) {
		code := commandExit.ExitCode()
		if code < 1 || code > 255 {
			code = 1
		}
		return exitError{code: code, err: fmt.Errorf("unit coverage exited with status %d", commandExit.ExitCode())}
	}
	return fmt.Errorf("run unit coverage: %w", commandErr)
}

type commandInvocation struct {
	name         string
	args         []string
	dir          string
	stdoutWriter io.Writer
	stderrWriter io.Writer
}

type commandRunnerFunc func(commandInvocation) error

var commandRunner commandRunnerFunc = func(invocation commandInvocation) error {
	cmd := exec.Command(invocation.name, invocation.args...)
	cmd.Dir = invocation.dir
	cmd.Stdout = invocation.stdoutWriter
	cmd.Stderr = invocation.stderrWriter
	return cmd.Run()
}

func coverageCommandArgs(cfg config) []string {
	args := []string{
		"run", "./cmd/gocoveragecheck",
		"-suite", "unit",
		"-min", strconv.FormatFloat(cfg.minimumCoverage, 'f', -1, 64),
		"-package-manifest", cfg.packageManifest,
		"-package-floor-policy", cfg.packageFloorPolicy,
		"-timeout", cfg.testTimeout,
		"-profile", cfg.profilePath,
		"-json-output", cfg.coveragePath,
		"-timing-output", cfg.timingPath,
	}
	if cfg.jobsSet || cfg.jobs != 0 {
		args = append(args, "-jobs", strconv.Itoa(cfg.jobs))
	}
	if strings.TrimSpace(cfg.coverageDiagnosticsPath) != "" {
		args = append(args, "-coverage-build-diagnostics-output", cfg.coverageDiagnosticsPath)
	}
	return args
}

func ownedArtifactPaths(cfg config) []string {
	paths := []string{cfg.logPath, cfg.profilePath, cfg.coveragePath, cfg.timingPath}
	if strings.TrimSpace(cfg.coverageDiagnosticsPath) != "" {
		paths = append(paths, cfg.coverageDiagnosticsPath)
	}
	return paths
}

func validateConfig(cfg config) error {
	if cfg.jobsSet || cfg.jobs != 0 {
		if cfg.jobs <= 0 {
			return fmt.Errorf("jobs must be a positive integer (got %d)", cfg.jobs)
		}
	}
	for name, value := range map[string]string{
		"profile":          cfg.profilePath,
		"coverage-summary": cfg.coveragePath,
		"timing-summary":   cfg.timingPath,
		"log":              cfg.logPath,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s path is required", name)
		}
	}
	return nil
}

func renderConsoleSummary(coveragePath, timingPath string, output io.Writer) error {
	var coverage coverageSummary
	if err := decodeJSONFile(coveragePath, &coverage); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var timing timingSummary
	if err := decodeJSONFile(timingPath, &timing); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	slices.SortFunc(coverage.Packages, func(left, right packageCoverage) int {
		return strings.Compare(left.Package, right.Package)
	})
	slices.SortFunc(timing.Packages, func(left, right packageTiming) int {
		return strings.Compare(left.Package, right.Package)
	})

	if len(coverage.Packages) > 0 {
		_, _ = fmt.Fprintln(output, "Unit coverage for pkg/:")
		for _, pkg := range coverage.Packages {
			if display, ok := pkgDisplayPath(pkg.Package); ok {
				_, _ = fmt.Fprintf(output, "%s %.1f%%\n", display, pkg.CoveragePercent)
			}
		}
	}
	wroteTiming := false
	for _, pkg := range timing.Packages {
		display, ok := pkgDisplayPath(pkg.Package)
		if !ok {
			continue
		}
		if !wroteTiming {
			if len(coverage.Packages) > 0 {
				_, _ = fmt.Fprintln(output)
			}
			_, _ = fmt.Fprintln(output, "Unit package latencies:")
			wroteTiming = true
		}
		_, _ = fmt.Fprintf(output, "%s %.3fs\n", display, pkg.Seconds)
	}
	return nil
}

func decodeJSONFile(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func pkgDisplayPath(importPath string) (string, bool) {
	const marker = "/pkg/"
	index := strings.Index(importPath, marker)
	if index < 0 {
		return "", false
	}
	return importPath[index+1:], true
}
