package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

type budgetConfig struct {
	budgetPath string
	samples    string
	mode       string
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
	flag.StringVar(&cfg.mode, "mode", "final", "validation mode: final or baseline")
	flag.Parse()
	return cfg
}
