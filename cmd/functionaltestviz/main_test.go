package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRequiresCoverageSummary(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(config{
		repositoryRoot: t.TempDir(),
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "coverage-summary path is required") {
		t.Fatalf("run() error = %v, want required coverage-summary guidance", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestValidateSuiteConfigRequiresBoundedRawCaptureConfiguration(t *testing.T) {
	t.Parallel()
	base := config{
		coverageSummaryPath: "coverage.json",
		timingSummaryPath:   "timing.json",
		outputPath:          "tests.md",
		logPath:             "command.log",
		profilePath:         "coverage.out",
		jobs:                12,
	}
	if err := validateSuiteConfig(base); err != nil {
		t.Fatalf("legacy suite configuration: %v", err)
	}
	base.rawFailureDir = ".artifacts/functional-test-viz/raw-failures"
	if err := validateSuiteConfig(base); err == nil || !strings.Contains(err.Error(), "raw-failure-max-bytes") {
		t.Fatalf("raw directory without size cap error = %v, want bounded cap requirement", err)
	}
	base.rawFailureMaxBytes = 536870912
	if err := validateSuiteConfig(base); err != nil {
		t.Fatalf("valid bounded raw configuration: %v", err)
	}
}

func TestWriteCompactVerdictPreservesFailedCoverageExit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	logPath := filepath.Join(root, "command.log")
	verdictPath := filepath.Join(root, "functional-coverage-verdict.txt")
	log := "coverage not evaluated: 1 failed tests observed; package floors were NOT checked because the coverage test run failed\n"
	if err := os.WriteFile(logPath, []byte(log), 0o644); err != nil {
		t.Fatalf("write failed functional command log: %v", err)
	}
	if err := writeCompactVerdict(logPath, verdictPath, 1); err != nil {
		t.Fatalf("write compact failed verdict: %v", err)
	}
	verdict, err := os.ReadFile(verdictPath)
	if err != nil {
		t.Fatalf("read compact failed verdict: %v", err)
	}
	if normalizedExitCode(1) != 1 || !strings.Contains(string(verdict), "Functional coverage outcome: test-failure") || !strings.Contains(string(verdict), strings.TrimSpace(log)) {
		t.Fatalf("failed verdict = %q, want test-failure details and nonzero exit", verdict)
	}
}

func TestCoverageCommandArgumentsKeepsProbeOptIn(t *testing.T) {
	t.Parallel()

	base := config{
		jobs:                8,
		minimumCoverage:     33.1,
		packageManifestPath: "coverage-minimums.json",
		packageFloorPolicy:  "blocking",
		quarantinePath:      "quarantine.json",
		testTimeout:         "10m",
		profilePath:         "coverage.out",
		coverageSummaryPath: "coverage-summary.json",
		timingSummaryPath:   "timing-summary.json",
	}
	defaultArgs := coverageCommandArguments(base)
	if slicesContainsString(defaultArgs, "-coverage-build-diagnostics-output") {
		t.Fatalf("default coverage args = %v, want probe flag omitted", defaultArgs)
	}

	withProbe := base
	withProbe.coverageBuildDiagnosticsPath = "coverage-build-diagnostics.json"
	probeArgs := coverageCommandArguments(withProbe)
	if !slicesContainsString(probeArgs, "-coverage-build-diagnostics-output") || !slicesContainsString(probeArgs, "coverage-build-diagnostics.json") {
		t.Fatalf("probe coverage args = %v, want optional diagnostic path", probeArgs)
	}

	withRawEvidence := base
	withRawEvidence.coverageTestPackages = "./cmd/gocoveragecheck/testdata/rawfailure"
	withRawEvidence.coverageCoverPackages = "github.com/portpowered/infinite-you/cmd/gocoveragecheck/testdata/rawfailure"
	withRawEvidence.rawFailureDir = ".artifacts/functional-test-viz/raw-failures"
	withRawEvidence.rawFailureMaxBytes = 536870912
	rawArgs := coverageCommandArguments(withRawEvidence)
	for _, expected := range []string{
		"-packages", withRawEvidence.coverageTestPackages,
		"-coverpkg", withRawEvidence.coverageCoverPackages,
		"-raw-failure-dir", withRawEvidence.rawFailureDir,
		"-raw-failure-max-bytes", "536870912",
	} {
		if !slicesContainsString(rawArgs, expected) {
			t.Fatalf("raw coverage args = %v, want %q", rawArgs, expected)
		}
	}
}

func slicesContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRenderConsoleSummaryOnlyPrintsPkgCoverageAndFunctionalPackageLatencies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	coveragePath := filepath.Join(root, "coverage.json")
	timingPath := filepath.Join(root, "timing.json")
	if err := os.WriteFile(coveragePath, []byte(`{"packages":[{"package":"example/pkg/alpha","coveragePercent":80},{"package":"example/cmd/tool","coveragePercent":90}]}`), 0o644); err != nil {
		t.Fatalf("write coverage fixture: %v", err)
	}
	if err := os.WriteFile(timingPath, []byte(`{"packages":[{"package":"example/tests/functional/work","seconds":0.125,"outcome":"fail"},{"package":"example/pkg/alpha","seconds":0.001,"outcome":"pass"}],"tests":[{"package":"example/tests/functional/work","test":"TestWork","seconds":0.100,"outcome":"fail"}]}`), 0o644); err != nil {
		t.Fatalf("write timing fixture: %v", err)
	}

	var output bytes.Buffer
	if err := renderConsoleSummary(coveragePath, timingPath, &output); err != nil {
		t.Fatalf("render console summary: %v", err)
	}
	got := output.String()
	for _, expected := range []string{"pkg/alpha 80.0%", "Functional package latencies:", "tests/functional/work 0.125s"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("console summary missing %q:\n%s", expected, got)
		}
	}
	for _, unwanted := range []string{"cmd/tool", "TestWork", "pass", "fail"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("console summary contains unwanted %q:\n%s", unwanted, got)
		}
	}
}

func TestPublishFunctionalJobSummaryUsesGeneratedMarkdown(t *testing.T) {
	root := t.TempDir()
	markdownPath := filepath.Join(root, "functional-tests.md")
	summaryPath := filepath.Join(root, "github-summary.md")
	if err := os.WriteFile(markdownPath, []byte("# Functional tests\n\nGenerated by Go.\n"), 0o644); err != nil {
		t.Fatalf("write Markdown fixture: %v", err)
	}
	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
	if err := publishFunctionalJobSummary(config{outputPath: markdownPath, logPath: filepath.Join(root, "command.log")}); err != nil {
		t.Fatalf("publish functional job summary: %v", err)
	}
	published, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("read published summary: %v", err)
	}
	if string(published) != "# Functional tests\n\nGenerated by Go.\n" {
		t.Fatalf("published summary = %q", published)
	}
}

func TestRunRequiresTimingSummary(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	coveragePath := filepath.Join(root, "coverage-summary.json")
	if err := os.WriteFile(coveragePath, []byte(`{"coveredStatements":0,"measurableStatements":0,"coveragePercent":0.0,"packages":[]}`), 0o644); err != nil {
		t.Fatalf("write coverage summary: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := run(config{
		repositoryRoot:      root,
		coverageSummaryPath: coveragePath,
	}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "timing-summary path is required") {
		t.Fatalf("run() error = %v, want required timing-summary guidance", err)
	}
}

func TestRunWritesCatalog(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	functionalRoot := filepath.Join(root, "tests", "functional", "transport")
	if err := os.MkdirAll(functionalRoot, 0o755); err != nil {
		t.Fatalf("mkdir functional root: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(functionalRoot, "help_test.go"),
		[]byte("package transport\n\nimport \"testing\"\n\n// TestHelp verifies help.\nfunc TestHelp(t *testing.T) {}\n"),
		0o644,
	); err != nil {
		t.Fatalf("write fixture test: %v", err)
	}

	coveragePath := filepath.Join(root, "coverage-summary.json")
	const coverage = `{
  "coveredStatements": 0,
  "measurableStatements": 0,
  "coveragePercent": 0.0,
  "packages": []
}
`
	if err := os.WriteFile(coveragePath, []byte(coverage), 0o644); err != nil {
		t.Fatalf("write coverage summary: %v", err)
	}

	timingPath := filepath.Join(root, "functional-timing-summary.json")
	const timing = `{
  "version": 1,
  "complete": true,
  "wallSeconds": 0.0,
  "packageElapsedSecondsSum": 0.0,
  "packageCount": 0,
  "packages": []
}
`
	if err := os.WriteFile(timingPath, []byte(timing), 0o644); err != nil {
		t.Fatalf("write timing summary: %v", err)
	}

	outputPath := filepath.Join(root, "out", "functional-tests.md")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run(config{
		repositoryRoot:      root,
		coverageSummaryPath: coveragePath,
		timingSummaryPath:   timingPath,
		outputPath:          outputPath,
	}, &stdout, &stderr); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if stdout.String() != "ok\n" {
		t.Fatalf("stdout = %q, want ok", stdout.String())
	}
	if !strings.Contains(stderr.String(), "wrote catalog to") {
		t.Fatalf("stderr = %q, want wrote-catalog message", stderr.String())
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(got), "TestHelp") {
		t.Fatalf("output missing TestHelp:\n%s", got)
	}
}
