package main

import (
	"flag"
	"fmt"
)

const testJobsFlagUsage = "positive instrumented go test -p override; omitted uses -jobs (functional subprocess I/O wait can benefit from controlled oversubscription)"

func registerTestJobsFlag(cfg *config) {
	flag.IntVar(&cfg.testJobsOverride, "test-jobs", 0, testJobsFlagUsage)
}

func markTestJobsOverrideSet(cfg *config) {
	flag.Visit(func(visited *flag.Flag) {
		if visited.Name == "test-jobs" {
			cfg.testJobsOverrideSet = true
		}
	})
}

func validateTestJobsOverride(cfg config) error {
	if cfg.testJobsOverride < 0 || (cfg.testJobsOverride == 0 && cfg.testJobsOverrideSet) {
		return fmt.Errorf("configure go coverage: -test-jobs must be a positive integer (got %d)", cfg.testJobsOverride)
	}
	return nil
}

// instrumentedTestJobs keeps the instrumented test window independently
// tunable: functional subprocesses spend time waiting on I/O, so controlled
// oversubscription can reduce wall time without changing discovery's jobs.
func (cfg config) instrumentedTestJobs(targetOS string, logicalCPUs int) int {
	if cfg.testJobsOverride > 0 {
		return cfg.testJobsOverride
	}
	return cfg.testJobs(targetOS, logicalCPUs)
}
