package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRenderConsoleSummaryOnlyPrintsPkgCoverageAndPackageLatencies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	coveragePath := filepath.Join(root, "coverage.json")
	timingPath := filepath.Join(root, "timing.json")
	if err := os.WriteFile(coveragePath, []byte(`{"packages":[{"package":"example/pkg/alpha","coveragePercent":80},{"package":"example/cmd/tool","coveragePercent":90}]}`), 0o644); err != nil {
		t.Fatalf("write coverage fixture: %v", err)
	}
	if err := os.WriteFile(timingPath, []byte(`{"packages":[{"package":"example/pkg/alpha","seconds":0.125,"outcome":"fail"},{"package":"example/cmd/tool","seconds":0.001,"outcome":"pass"}]}`), 0o644); err != nil {
		t.Fatalf("write timing fixture: %v", err)
	}

	var output bytes.Buffer
	if err := renderConsoleSummary(coveragePath, timingPath, &output); err != nil {
		t.Fatalf("render console summary: %v", err)
	}
	got := output.String()
	for _, expected := range []string{"Unit coverage for pkg/:", "pkg/alpha 80.0%", "Unit package latencies:", "pkg/alpha 0.125s"} {
		if !strings.Contains(got, expected) {
			t.Fatalf("console summary missing %q:\n%s", expected, got)
		}
	}
	for _, unwanted := range []string{"cmd/tool", "pass", "fail"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("console summary contains unwanted %q:\n%s", unwanted, got)
		}
	}
}

func TestValidateConfigRequiresOwnedArtifacts(t *testing.T) {
	t.Parallel()
	if err := validateConfig(config{}); err == nil {
		t.Fatal("validateConfig() error = nil, want missing artifact failure")
	}
}

func TestCoverageCommandArgsPreserveDefaultCompatibility(t *testing.T) {
	t.Parallel()
	cfg := unitCoverageTestConfig(t)
	want := []string{
		"run", "./cmd/gocoveragecheck",
		"-suite", "unit",
		"-min", "75.9",
		"-package-manifest", "manifest.json",
		"-package-floor-policy", "blocking",
		"-timeout", "10m",
		"-profile", cfg.profilePath,
		"-json-output", cfg.coveragePath,
		"-timing-output", cfg.timingPath,
	}
	if got := coverageCommandArgs(cfg); !slices.Equal(got, want) {
		t.Fatalf("coverageCommandArgs() = %v, want %v", got, want)
	}
}

func TestRunForwardsOptionalCoverageOptionsOnceAndCleansDiagnostic(t *testing.T) {
	cfg := unitCoverageTestConfig(t)
	cfg.jobs = 4
	cfg.jobsSet = true
	cfg.coverageDiagnosticsPath = filepath.Join(filepath.Dir(cfg.logPath), "coverage-build-diagnostics.json")
	if err := os.WriteFile(cfg.coverageDiagnosticsPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale diagnostic: %v", err)
	}

	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	var invocations []commandInvocation
	commandRunner = func(invocation commandInvocation) error {
		invocations = append(invocations, invocation)
		if _, err := os.Stat(cfg.coverageDiagnosticsPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("stale diagnostic stat error = %v, want removed before child invocation", err)
		}
		return nil
	}

	if err := run(cfg, new(bytes.Buffer)); err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if len(invocations) != 1 {
		t.Fatalf("child invocation count = %d, want one", len(invocations))
	}
	if got, want := invocations[0].args, coverageCommandArgs(cfg); !slices.Equal(got, want) {
		t.Fatalf("child args = %v, want %v", got, want)
	}
}

func TestRunPreservesChildExitStatus(t *testing.T) {
	cfg := unitCoverageTestConfig(t)
	originalRunner := commandRunner
	t.Cleanup(func() { commandRunner = originalRunner })
	commandRunner = func(commandInvocation) error {
		return exitError{code: 23, err: errors.New("coverage floor failed")}
	}

	err := run(cfg, new(bytes.Buffer))
	if err == nil || !strings.Contains(err.Error(), "coverage floor failed") {
		t.Fatalf("run() error = %v, want child failure", err)
	}
	var status exitError
	if !errors.As(err, &status) || status.code != 23 {
		t.Fatalf("run() status = %#v, want exit code 23", status)
	}
}

func TestJobsValidationRejectsExplicitNonPositiveValues(t *testing.T) {
	t.Parallel()
	for _, jobs := range []int{0, -1} {
		cfg := unitCoverageTestConfig(t)
		cfg.jobs = jobs
		cfg.jobsSet = true
		if err := validateConfig(cfg); err == nil {
			t.Fatalf("validateConfig(jobs=%d) error = nil, want positive-integer failure", jobs)
		}
	}
}

func TestParseConfigRejectsMalformedJobsAndTracksExplicitValue(t *testing.T) {
	t.Parallel()
	cfg, err := parseConfigArgs([]string{"-jobs", "4"})
	if err != nil {
		t.Fatalf("parseConfigArgs() error = %v", err)
	}
	if cfg.jobs != 4 || !cfg.jobsSet {
		t.Fatalf("parsed jobs = %d, jobsSet = %t, want 4 and true", cfg.jobs, cfg.jobsSet)
	}
	if _, err := parseConfigArgs([]string{"-jobs", "not-a-number"}); err == nil {
		t.Fatal("parseConfigArgs(malformed jobs) error = nil, want parse failure")
	}
}

func unitCoverageTestConfig(t *testing.T) config {
	t.Helper()
	root := t.TempDir()
	return config{
		goBinary:           "go",
		repositoryRoot:     root,
		minimumCoverage:    75.9,
		packageManifest:    "manifest.json",
		packageFloorPolicy: "blocking",
		testTimeout:        "10m",
		profilePath:        filepath.Join(root, "coverage.out"),
		coveragePath:       filepath.Join(root, "coverage-summary.json"),
		timingPath:         filepath.Join(root, "timing-summary.json"),
		logPath:            filepath.Join(root, "command.log"),
	}
}
