package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	Name           string `json:"name"`
	Status         string `json:"status"`
	DurationMillis int64  `json:"durationMillis"`
	Output         string `json:"output"`
	Error          string `json:"error,omitempty"`
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
