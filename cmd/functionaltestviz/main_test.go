package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestRunFunctionalSuiteCoverageAndFailureHandoff(t *testing.T) {
	for _, tc := range []struct {
		name         string
		coverageCode int
		renderCode   int
		handoff      bool
		wantCode     int
		wantCommands int
	}{
		{"success", 0, 0, false, 0, 2},
		{"coverage failure", 1, 0, false, 1, 1},
		{"coverage failure handoff", 1, 0, true, 0, 2},
		{"coverage infrastructure failure", 2, 0, true, 2, 1},
		{"renderer failure", 0, 3, true, 3, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := functionalSuiteTestConfig(t)
			if tc.handoff {
				cfg.exitCodePath = filepath.Join(filepath.Dir(cfg.logPath), "exit.txt")
			}
			var commands [][]string
			runner := func(name string, args []string, log io.Writer) (int, error) {
				if name != cfg.goBinary {
					t.Fatalf("executable = %q, want %q", name, cfg.goBinary)
				}
				commands = append(commands, slices.Clone(args))
				code := tc.renderCode
				if len(commands) == 1 {
					if !slices.Equal(args, coverageCommandArguments(cfg)) {
						t.Fatalf("first command = %v, want coverage directly", args)
					}
					code = tc.coverageCode
					writeSuiteTestCoverage(t, cfg, log)
				} else {
					if len(args) < 2 || args[1] != "./cmd/functionaltestviz" {
						t.Fatalf("second command = %v, want renderer", args)
					}
					if err := writeTextFile(cfg.outputPath, "Rendered functional coverage\n"); err != nil {
						t.Fatal(err)
					}
				}
				if code != 0 {
					return code, fmt.Errorf("command failed: %d", code)
				}
				return 0, nil
			}
			var stdout bytes.Buffer
			err := runFunctionalSuite(cfg, &stdout, io.Discard, runner)
			assertSuiteTestExit(t, err, tc.wantCode)
			if len(commands) != tc.wantCommands {
				t.Fatalf("commands = %v, want %d", commands, tc.wantCommands)
			}
			assertSuiteTestArtifacts(t, cfg, stdout.String(), tc.coverageCode, tc.wantCode)
		})
	}
}

func assertSuiteTestExit(t *testing.T, err error, wantCode int) {
	t.Helper()
	if wantCode == 0 && err != nil {
		t.Fatalf("run suite: %v", err)
	}
	if wantCode != 0 {
		var exitErr suiteExitError
		if !errors.As(err, &exitErr) || exitErr.code != wantCode {
			t.Fatalf("error = %v, want suite exit %d", err, wantCode)
		}
	}
}

func functionalSuiteTestConfig(t *testing.T) config {
	t.Helper()
	root := t.TempDir()
	t.Setenv("GITHUB_STEP_SUMMARY", filepath.Join(root, "job-summary.md"))
	return config{
		goBinary: "test-go", jobs: 2,
		coverageSummaryPath: filepath.Join(root, "coverage.json"),
		timingSummaryPath:   filepath.Join(root, "timing.json"),
		outputPath:          filepath.Join(root, "tests.md"),
		logPath:             filepath.Join(root, "command.log"),
		profilePath:         filepath.Join(root, "coverage.out"),
		verdictPath:         filepath.Join(root, "verdict.txt"),
	}
}

func writeSuiteTestCoverage(t *testing.T, cfg config, log io.Writer) {
	t.Helper()
	for _, path := range []string{cfg.coverageSummaryPath, cfg.timingSummaryPath} {
		if err := writeTextFile(path, `{"packages":[{"package":"github.com/portpowered/infinite-you/pkg/example","coveragePercent":42,"seconds":1}]}`); err != nil {
			t.Fatal(err)
		}
	}
	_, _ = fmt.Fprintln(log, "coverage not evaluated: package floors were NOT checked")
}

func assertSuiteTestArtifacts(t *testing.T, cfg config, stdout string, coverageCode, suiteCode int) {
	t.Helper()
	if !strings.Contains(stdout, "pkg/example 42.0%") {
		t.Fatalf("console report missing: %q", stdout)
	}
	summary, err := os.ReadFile(os.Getenv("GITHUB_STEP_SUMMARY"))
	if err != nil || len(summary) == 0 {
		t.Fatalf("published job summary = %q, error = %v", summary, err)
	}
	if cfg.exitCodePath == "" {
		return
	}
	exit, err := os.ReadFile(cfg.exitCodePath)
	if err != nil || string(exit) != strconv.Itoa(coverageCode)+"\n" {
		t.Fatalf("coverage exit handoff = %q, error = %v", exit, err)
	}
	if suiteCode == 0 {
		verdict, err := os.ReadFile(cfg.verdictPath)
		outcome := "green"
		if coverageCode != 0 {
			outcome = "test-failure"
		}
		if err != nil || !strings.Contains(string(verdict), "Functional coverage outcome: "+outcome) {
			t.Fatalf("verdict = %q, error = %v", verdict, err)
		}
	}
}

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

func TestWriteCompactVerdictCompleteLinesAndOutcomes(t *testing.T) {
	t.Parallel()
	longSelected := "  package=" + strings.Repeat("x", 70*1024)
	for _, tc := range []struct {
		name, log, selected, outcome string
		code                         int
	}{
		{"long lines", "Functional suite inventory: before\r\n" + strings.Repeat("diagnostic", 8192) + "\n" + longSelected + "\r\nFunctional package coverage verdict: final",
			"Functional suite inventory: before\n" + longSelected + "\nFunctional package coverage verdict: final", "green", 0},
		{"advisory", "!!! COVERAGE FLOOR POLICY: advisory !!!\npackage coverage regression: example\n", "!!! COVERAGE FLOOR POLICY: advisory !!!\npackage coverage regression: example", "advisory", 0},
		{"test failure", "coverage not evaluated: package floors were NOT checked\n", "coverage not evaluated: package floors were NOT checked", "test-failure", 1},
		{"coverage failure", "  floor violation: example\n", "  floor violation: example", "coverage-gate-failure", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			logPath, verdictPath := filepath.Join(root, "log"), filepath.Join(root, "verdict")
			if err := os.WriteFile(logPath, []byte(tc.log), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := writeCompactVerdict(logPath, verdictPath, tc.code); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(verdictPath)
			if err != nil {
				t.Fatal(err)
			}
			want := "Functional coverage outcome: " + tc.outcome + "\n" + tc.selected + "\n"
			if string(got) != want {
				t.Fatalf("verdict differs: got length=%d want=%d", len(got), len(want))
			}
		})
	}
}

func TestWriteCompactVerdictMissingOrEmptyLog(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, log, want string
		missing         bool
	}{
		{"missing", "", "open functional coverage log", true},
		{"empty", "", "functional coverage verdict extract is empty", false},
		{"unselected", "unselected diagnostic\n", "functional coverage verdict extract is empty", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			logPath, verdictPath := filepath.Join(root, "log"), filepath.Join(root, "verdict")
			if !tc.missing {
				if err := os.WriteFile(logPath, []byte(tc.log), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err := writeCompactVerdict(logPath, verdictPath, 0)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
			if _, err := os.Stat(verdictPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed extraction published verdict: %v", err)
			}
		})
	}
}

type compactVerdictErrorReader struct{ err error }

func (reader compactVerdictErrorReader) Read([]byte) (int, error) { return 0, reader.err }

func TestReadCompactVerdictLinesPropagatesReadError(t *testing.T) {
	t.Parallel()
	readErr := errors.New("controlled log read failure")
	for _, prefix := range []string{"", "Functional suite inventory: selected\n", "Functional suite inventory: partial"} {
		input := io.MultiReader(strings.NewReader(prefix), compactVerdictErrorReader{err: readErr})
		lines, err := readCompactVerdictLines(input)
		if !errors.Is(err, readErr) || lines != nil {
			t.Fatalf("lines=%v error=%v, want no verdict and original read error", lines, err)
		}
	}
}
