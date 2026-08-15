package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type lintReport struct {
	Version             int                `json:"version"`
	Jobs                int                `json:"jobs"`
	TotalDurationMillis int64              `json:"totalDurationMillis"`
	Targets             []lintReportTarget `json:"targets"`
}

type lintReportTarget struct {
	Name                 string `json:"name"`
	Status               string `json:"status"`
	DurationMillis       int64  `json:"durationMillis"`
	Output               string `json:"output"`
	Error                string `json:"error,omitempty"`
	ViolationCount       *int   `json:"violationCount,omitempty"`
	ViolationCountSource string `json:"violationCountSource,omitempty"`
}

const violationCountMarker = "LINT_VIOLATION_COUNT:"

func checkerViolationCount(output string) (int, bool) {
	var count *int
	for _, line := range strings.Split(output, "\n") {
		value, ok := strings.CutPrefix(strings.TrimSpace(line), violationCountMarker)
		if !ok {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < 0 || count != nil {
			return 0, false
		}
		count = &parsed
	}
	if count == nil {
		return 0, false
	}
	return *count, true
}

func writeReportFile(path string, jobs int, total time.Duration, results []targetResult) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("lint report file must not be empty")
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("create lint report directory: %w", err)
	}

	report := lintReport{
		Version:             1,
		Jobs:                jobs,
		TotalDurationMillis: total.Milliseconds(),
		Targets:             make([]lintReportTarget, 0, len(results)),
	}
	for _, result := range results {
		target := lintReportTarget{
			Name:           result.target,
			Status:         reportStatus(result.err),
			DurationMillis: result.duration.Milliseconds(),
			Output:         result.output,
		}
		if result.err == nil {
			count := 0
			target.ViolationCount = &count
			target.ViolationCountSource = "successful-check"
		} else if count, ok := checkerViolationCount(result.output); ok {
			target.ViolationCount = &count
			target.ViolationCountSource = "checker-marker"
		}
		if result.err != nil {
			target.Error = result.err.Error()
		}
		report.Targets = append(report.Targets, target)
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode lint report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write lint report %s: %w", path, err)
	}
	return nil
}

func reportStatus(err error) string {
	if err != nil {
		return "fail"
	}
	return "pass"
}
