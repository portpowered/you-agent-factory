package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

type config struct {
	runSuite                     bool
	goBinary                     string
	repositoryRoot               string
	functionalRoot               string
	coverageSummaryPath          string
	timingSummaryPath            string
	outputPath                   string
	logPath                      string
	profilePath                  string
	coverageBuildDiagnosticsPath string
	verdictPath                  string
	exitCodePath                 string
	tier                         string
	trigger                      string
	budget                       string
	short                        bool
	quarantinePath               string
	jobs                         int
	minimumCoverage              float64
	packageManifestPath          string
	packageFloorPolicy           string
	testTimeout                  string
}

func main() {
	cfg := parseConfig()
	if err := run(cfg, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr suiteExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.code)
		}
		os.Exit(1)
	}
}

func parseConfig() config {
	var cfg config
	flag.BoolVar(&cfg.runSuite, "run-suite", false, "run functional coverage, generate artifacts, and print the compact console report")
	flag.StringVar(&cfg.goBinary, "go", "go", "Go executable used by -run-suite")
	flag.StringVar(&cfg.repositoryRoot, "root", ".", "repository root used to resolve golden manifests and default paths")
	flag.StringVar(&cfg.functionalRoot, "functional-root", "", "functional test tree to inventory (default: <root>/tests/functional)")
	flag.StringVar(&cfg.coverageSummaryPath, "coverage-summary", "", "path to gocoveragecheck coverage-summary JSON (required)")
	flag.StringVar(&cfg.timingSummaryPath, "timing-summary", "", "path to gocoveragecheck functional-timing-summary JSON (required)")
	flag.StringVar(&cfg.outputPath, "output", "", "Markdown output path (default: <root>/.artifacts/functional-test-viz/functional-tests.md)")
	flag.StringVar(&cfg.logPath, "log", "", "complete command log path used by -run-suite")
	flag.StringVar(&cfg.profilePath, "profile", "", "coverage profile path used by -run-suite")
	flag.StringVar(&cfg.coverageBuildDiagnosticsPath, "coverage-build-diagnostics", "", "optional coverage compile-probe cache diagnostic path used by -run-suite")
	flag.StringVar(&cfg.verdictPath, "verdict", "", "compact functional coverage verdict path used by -run-suite")
	flag.StringVar(&cfg.exitCodePath, "exit-code-file", "", "optional gocoveragecheck exit-code handoff path")
	flag.StringVar(&cfg.tier, "tier", "pr-short", "functional test tier label")
	flag.StringVar(&cfg.trigger, "trigger", "local", "functional test trigger label")
	flag.StringVar(&cfg.budget, "budget", "35m", "functional test budget label")
	flag.BoolVar(&cfg.short, "short", true, "run the short functional tier")
	flag.StringVar(&cfg.quarantinePath, "quarantine", "tests/functional/functional-quarantine.json", "functional quarantine manifest")
	flag.IntVar(&cfg.jobs, "jobs", 2, "functional package concurrency")
	flag.Float64Var(&cfg.minimumCoverage, "minimum", 33.1, "minimum aggregate functional coverage")
	flag.StringVar(&cfg.packageManifestPath, "package-manifest", "docs/internal/baselines/go-functional-coverage-package-minimums.json", "functional package-floor manifest")
	flag.StringVar(&cfg.packageFloorPolicy, "package-floor-policy", "blocking", "functional package-floor policy")
	flag.StringVar(&cfg.testTimeout, "test-timeout", "10m", "gocoveragecheck test timeout")
	flag.Parse()
	return cfg
}

func run(cfg config, stdout, stderr io.Writer) error {
	if cfg.runSuite {
		return runFunctionalSuite(cfg, stdout, stderr)
	}
	generateCfg := functionaltestviz.GenerateConfig{
		RepositoryRoot:      cfg.repositoryRoot,
		FunctionalRoot:      cfg.functionalRoot,
		CoverageSummaryPath: cfg.coverageSummaryPath,
		TimingSummaryPath:   cfg.timingSummaryPath,
		OutputPath:          cfg.outputPath,
	}
	if err := functionaltestviz.Generate(generateCfg); err != nil {
		return err
	}
	outputPath := cfg.outputPath
	if outputPath == "" {
		root := cfg.repositoryRoot
		if root == "" {
			root = "."
		}
		outputPath = filepath.Join(root, filepath.FromSlash(functionaltestviz.DefaultOutputPath))
	}
	_, _ = fmt.Fprintf(stderr, "[agent-factory:functional-test-viz] wrote catalog to %s\n", outputPath)
	_, _ = fmt.Fprintln(stdout, "ok")
	return nil
}
