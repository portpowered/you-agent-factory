package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecuteWritesDeterministicJSONCoverageTotals(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	cfg := config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages:   "./pkg/config",
		jsonOutput: jsonPath,
	}
	if err := execute(cfg); err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if err := execute(cfg); err != nil {
		t.Fatalf("second execute() error = %v", err)
	}

	first, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json: %v", err)
	}
	if err := execute(cfg); err != nil {
		t.Fatalf("third execute() error = %v", err)
	}
	second, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read coverage summary json after rerun: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("coverage summary json was not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}

	var summary coverageSummaryJSON
	if err := json.Unmarshal(first, &summary); err != nil {
		t.Fatalf("decode coverage summary json: %v\n%s", err, first)
	}
	if summary.CoveredStatements != 8 {
		t.Fatalf("coveredStatements = %d, want 8", summary.CoveredStatements)
	}
	if summary.MeasurableStatements != 8 {
		t.Fatalf("measurableStatements = %d, want 8", summary.MeasurableStatements)
	}
	if summary.CoveragePercent != 100.0 {
		t.Fatalf("coveragePercent = %v, want 100.0", summary.CoveragePercent)
	}

	wantJSON := "{\n" +
		"  \"coveredStatements\": 8,\n" +
		"  \"measurableStatements\": 8,\n" +
		"  \"coveragePercent\": 100\n" +
		"}\n"
	if string(first) != wantJSON {
		t.Fatalf("coverage summary json = %q, want %q", first, wantJSON)
	}

	got := stdout.String()
	if !strings.Contains(got, "Go coverage 100.0% meets minimum 80.0%.") {
		t.Fatalf("execute() stdout = %q, want success message retained with json output", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestExecuteOmitsJSONFileWhenJSONOutputOptionAbsent(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	defer func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandPassing
	stdoutWriter = &stdout
	stderrWriter = &stderr

	jsonPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	err := execute(config{
		min: 80,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("execute() error = %v", err)
	}
	if _, err := os.Stat(jsonPath); !os.IsNotExist(err) {
		t.Fatalf("unexpected json file at %s with err=%v", jsonPath, err)
	}

	got := stdout.String()
	if !strings.Contains(got, "total: (statements) 100.0%") {
		t.Fatalf("execute() stdout = %q, want total coverage line", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/config\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/config", got)
	}
	if !strings.Contains(got, modulePath+"/pkg/service\tcoverage: 100.0% of statements") {
		t.Fatalf("execute() stdout = %q, want package summary for pkg/service", got)
	}
	wantSuccess := "Go coverage 100.0% meets minimum 80.0%."
	if !strings.Contains(got, wantSuccess) {
		t.Fatalf("execute() stdout = %q, want success message %q", got, wantSuccess)
	}
	if stderr.Len() != 0 {
		t.Fatalf("execute() stderr = %q, want empty stderr", stderr.String())
	}
}

func TestBuildCoverageSummaryJSONUsesMeasuredTotals(t *testing.T) {
	t.Parallel()

	result := coverageResult{
		actual: 75,
		packageTotals: map[string]packageCoverageTotals{
			modulePath + "/pkg/config":  {coveredStatements: 3, totalStatements: 4},
			modulePath + "/pkg/service": {coveredStatements: 0, totalStatements: 0},
		},
		packageSummaries: []packageCoverageSummary{
			{importPath: modulePath + "/pkg/config", coverage: 75},
			{importPath: modulePath + "/pkg/service", coverage: 0},
		},
	}

	summary := buildCoverageSummaryJSON(result)
	if summary.CoveredStatements != 3 {
		t.Fatalf("coveredStatements = %d, want 3", summary.CoveredStatements)
	}
	if summary.MeasurableStatements != 4 {
		t.Fatalf("measurableStatements = %d, want 4", summary.MeasurableStatements)
	}
	if summary.CoveragePercent != 75.0 {
		t.Fatalf("coveragePercent = %v, want 75.0", summary.CoveragePercent)
	}
}
