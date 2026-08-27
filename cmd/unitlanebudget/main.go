package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

type budgetConfig struct {
	budgetPath        string
	samples           string
	historicalSamples string
	referenceSamples  string
	manifest          string
	candidateCommit   string
	mode              string
}

var stdoutWriter io.Writer = os.Stdout
var stderrWriter io.Writer = os.Stderr

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(stderrWriter, err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseConfig()
	if cfg.mode != "final" && cfg.mode != "baseline" {
		return checkerError(fmt.Errorf("mode: expected final or baseline, actual %q", cfg.mode))
	}
	if cfg.mode == "baseline" {
		paths, err := splitSamplePaths(cfg.samples)
		if err != nil {
			return checkerError(err)
		}
		samples, err := loadTimingSamples(paths)
		if err != nil {
			return checkerError(err)
		}
		report, err := validateBaseline(samples)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdoutWriter, renderBudgetReport(report))
		return err
	}
	return runFinal(cfg)
}

func parseConfig() budgetConfig {
	cfg := budgetConfig{}
	flag.StringVar(&cfg.budgetPath, "budget", "docs/internal/baselines/go-unit-lane-latency-budget.v2.json", "unit-lane latency budget JSON path")
	flag.StringVar(&cfg.samples, "samples", "", "comma-separated candidate v2 timing summary paths")
	flag.StringVar(&cfg.historicalSamples, "historical-samples", "", "comma-separated retained historical v2 timing summary paths")
	flag.StringVar(&cfg.referenceSamples, "reference-samples", "", "comma-separated live reference-CI v2 timing summary paths")
	flag.StringVar(&cfg.manifest, "manifest", "", "reference-CI verdict manifest output path")
	flag.StringVar(&cfg.candidateCommit, "candidate-commit", "", "optional expected candidate commit; UNIT_CANDIDATE_COMMIT is used when set")
	flag.StringVar(&cfg.mode, "mode", "final", "validation mode: final or baseline")
	flag.Parse()
	if strings.TrimSpace(cfg.candidateCommit) == "" {
		cfg.candidateCommit = strings.TrimSpace(os.Getenv("UNIT_CANDIDATE_COMMIT"))
	}
	return cfg
}
