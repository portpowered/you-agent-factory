package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type budgetConfig struct {
	budgetPath      string
	samples         string
	mode            string
	root            string
	deadcodeReport  string
	skipUnitLatency bool
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
	if cfg.mode != "final" && cfg.mode != "baseline" && cfg.mode != "regenerate" {
		return checkerError(fmt.Errorf("mode: expected final, baseline, or regenerate, actual %q", cfg.mode))
	}
	if cfg.mode == "regenerate" {
		return regenerateSharedBaselines(cfg)
	}
	paths, err := splitSamplePaths(cfg.samples)
	if err != nil {
		return checkerError(err)
	}
	samples, err := loadTimingSamples(paths)
	if err != nil {
		return checkerError(err)
	}
	if cfg.mode == "baseline" {
		report, err := validateBaseline(samples)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(stdoutWriter, renderBudgetReport(report))
		return err
	}
	budget, err := loadLatencyBudget(cfg.budgetPath)
	if err != nil {
		return checkerError(err)
	}
	report, err := validateFinal(budget, samples)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdoutWriter, renderBudgetReport(report))
	return err
}

func parseConfig() budgetConfig {
	cfg := budgetConfig{}
	flag.StringVar(&cfg.budgetPath, "budget", "docs/internal/baselines/go-unit-lane-latency-budget.v1.json", "unit-lane latency budget JSON path")
	flag.StringVar(&cfg.samples, "samples", "", "comma-separated v2 timing summary paths")
	flag.StringVar(&cfg.mode, "mode", "final", "validation mode: final, baseline, or regenerate")
	flag.StringVar(&cfg.root, "root", ".", "repository root for regeneration inputs and outputs")
	flag.StringVar(&cfg.deadcodeReport, "deadcode-report", "", "normalized deadcode report to use; runs the pinned analyzer when omitted")
	flag.BoolVar(&cfg.skipUnitLatency, "skip-unit-latency", false, "leave the unit-latency budget unchanged during shared baseline regeneration")
	flag.Parse()
	return cfg
}
