package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunTargetsOverlapsWorkAndHonorsJobLimit(t *testing.T) {
	targets := []string{"one", "two", "three", "four", "five"}
	started := make(chan string, len(targets))
	release := make(chan struct{})
	var active atomic.Int32
	var peak atomic.Int32
	runner := func(_, target string, _, _ io.Writer) error {
		current := active.Add(1)
		for {
			previous := peak.Load()
			if current <= previous || peak.CompareAndSwap(previous, current) {
				break
			}
		}
		started <- target
		<-release
		active.Add(-1)
		return nil
	}

	finished := make(chan []targetResult, 1)
	go func() { finished <- runTargets("make", targets, 2, runner) }()
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("concurrent lint workers did not start")
		}
	}
	if got := peak.Load(); got != 2 {
		t.Fatalf("peak active targets = %d, want actual overlap at the job limit", got)
	}
	select {
	case <-finished:
		t.Fatal("lint lane finished before started targets were released")
	default:
	}
	close(release)
	select {
	case results := <-finished:
		if len(results) != len(targets) {
			t.Fatalf("terminal results = %d, want %d", len(results), len(targets))
		}
		if got := peak.Load(); got > 2 {
			t.Fatalf("peak active targets = %d, exceeded job limit 2", got)
		}
	case <-time.After(time.Second):
		t.Fatal("lint lane did not wait for all target results")
	}
}

func TestWriteReportAttributesMixedResultsAndProvidesReruns(t *testing.T) {
	results := runTargets("make", []string{"first", "second", "third"}, 3, func(_, target string, stdout, stderr io.Writer) error {
		fmt.Fprintf(stdout, "stdout:%s\n", target)
		fmt.Fprintf(stderr, "stderr:%s\n", target)
		if target == "first" || target == "third" {
			return errors.New("controlled failure")
		}
		return nil
	})

	var output bytes.Buffer
	err := writeReport(&output, results)
	if err == nil || !strings.Contains(err.Error(), "first, third") {
		t.Fatalf("writeReport() error = %v, want all failed targets", err)
	}
	text := output.String()
	for _, want := range []string{
		"===== lint target: first =====",
		"stdout:first",
		"stderr:first",
		"===== lint target: first: FAIL =====",
		"===== lint target: second: PASS =====",
		"===== lint target: third: FAIL =====",
		"LINT FAILED: 2 target(s)",
		"first (rerun: make first)",
		"third (rerun: make third)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("report = %q, want %q", text, want)
		}
	}
	if strings.Index(text, "lint target: first") > strings.Index(text, "lint target: second") || strings.Index(text, "lint target: second") > strings.Index(text, "lint target: third") {
		t.Fatalf("report target order = %q, want requested order", text)
	}
}

func TestRunSuccessReportsUnambiguousResult(t *testing.T) {
	original := executeTarget
	t.Cleanup(func() { executeTarget = original })
	executeTarget = func(_, target string, stdout, _ io.Writer) error {
		fmt.Fprintf(stdout, "completed:%s\n", target)
		return nil
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-make", "fixture-make", "-jobs", "1", "alpha", "beta"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run() exit code = %d, stderr = %q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "LINT PASSED: 2 target(s) completed successfully") {
		t.Fatalf("success report = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("success stderr = %q", stderr.String())
	}
}

func TestRunContinuesAfterEarlyFailureAndWritesCompleteJSONReport(t *testing.T) {
	original := executeTarget
	t.Cleanup(func() { executeTarget = original })
	var started []string
	executeTarget = func(_, target string, stdout, _ io.Writer) error {
		started = append(started, target)
		fmt.Fprintf(stdout, "diagnostic:%s\n", target)
		if target == "first" {
			fmt.Fprintln(stdout, "LINT_VIOLATION_COUNT: 1")
			return errors.New("controlled failure")
		}
		return nil
	}

	reportPath := filepath.Join(t.TempDir(), "backend-lint.json")
	var stdout, stderr bytes.Buffer
	if code := run([]string{"-report-file", reportPath, "-jobs", "1", "first", "second", "third"}, &stdout, &stderr); code == 0 {
		t.Fatalf("run() exit code = 0, want failed target; stdout = %q", stdout.String())
	}
	if !slices.Equal(started, []string{"first", "second", "third"}) {
		t.Fatalf("targets started = %v, want the runner to continue after first failure", started)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report lintReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v; data = %s", err, data)
	}
	if report.Version != 1 || report.Jobs != 1 || len(report.Targets) != 3 {
		t.Fatalf("report metadata = %+v, want version 1, jobs 1, and three targets", report)
	}
	for index, target := range report.Targets {
		if target.DurationMillis < 0 {
			t.Fatalf("target %q duration = %d, want a reported wall time", target.Name, target.DurationMillis)
		}
		if target.Name != started[index] {
			t.Fatalf("report target[%d] = %q, want execution order %q", index, target.Name, started[index])
		}
	}
	if report.Targets[0].Status != "fail" || report.Targets[0].Error != "controlled failure" {
		t.Fatalf("first target report = %+v", report.Targets[0])
	}
	if report.Targets[0].ViolationCount == nil || *report.Targets[0].ViolationCount != 1 || report.Targets[0].ViolationCountSource != "checker-marker" {
		t.Fatalf("first target violation count = %+v, want checker marker count 1", report.Targets[0])
	}
	for _, target := range report.Targets[1:] {
		if target.Status != "pass" || target.ViolationCount == nil || *target.ViolationCount != 0 || target.ViolationCountSource != "successful-check" {
			t.Fatalf("successful target report = %+v, want pass with zero violations", target)
		}
	}
}

func TestCheckerViolationCountRequiresOneNonnegativeMachineReadableMarker(t *testing.T) {
	for _, test := range []struct {
		name  string
		text  string
		count int
		valid bool
	}{
		{name: "valid", text: "diagnostic\nLINT_VIOLATION_COUNT: 4\n", count: 4, valid: true},
		{name: "missing", text: "diagnostic\n", valid: false},
		{name: "duplicate", text: "LINT_VIOLATION_COUNT: 1\nLINT_VIOLATION_COUNT: 1\n", valid: false},
		{name: "negative", text: "LINT_VIOLATION_COUNT: -1\n", valid: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			count, ok := checkerViolationCount(test.text)
			if ok != test.valid || (ok && count != test.count) {
				t.Fatalf("checkerViolationCount(%q) = (%d, %v), want (%d, %v)", test.text, count, ok, test.count, test.valid)
			}
		})
	}
}

func TestRunMakeTargetForwardsTargetAndEnvironment(t *testing.T) {
	original := execCommand
	t.Cleanup(func() { execCommand = original })
	t.Setenv("LINTLANE_HELPER", "forwarded")
	t.Setenv("GO_WANT_LINTLANE_HELPER", "1")
	execCommand = func(name string, args ...string) *exec.Cmd {
		helperArgs := append([]string{"-test.run=TestLintlaneHelperProcess", "--", name}, args...)
		return exec.Command(os.Args[0], helperArgs...)
	}

	var output bytes.Buffer
	if err := runMakeTarget("fixture-make", "pkg-maint", &output, &output); err != nil {
		t.Fatalf("runMakeTarget() error = %v; output = %q", err, output.String())
	}
	got := output.String()
	if !strings.HasPrefix(got, "helper target=fixture-make|--no-print-directory|") || !strings.Contains(got, "|pkg-maint env=forwarded\n") {
		t.Fatalf("helper output = %q", got)
	}
}

func TestLintlaneHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LINTLANE_HELPER") != "1" {
		return
	}
	args, ok := helperCommandArgs(os.Args)
	if !ok || len(args) < 1 {
		os.Exit(2)
	}
	fmt.Printf("helper target=%s env=%s\n", strings.Join(args, "|"), os.Getenv("LINTLANE_HELPER"))
	os.Exit(0)
}

func TestRunRejectsInvalidJobsBeforeCheckerExecution(t *testing.T) {
	for _, test := range []struct {
		name     string
		rawJobs  string
		wantCode int
	}{
		{name: "empty", rawJobs: "", wantCode: 2},
		{name: "whitespace only", rawJobs: " \t ", wantCode: 2},
		{name: "non numeric", rawJobs: "many", wantCode: 2},
		{name: "zero", rawJobs: "0", wantCode: 2},
		{name: "negative", rawJobs: "-1", wantCode: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := executeTarget
			t.Cleanup(func() { executeTarget = original })
			var executions atomic.Int32
			executeTarget = func(_, _ string, _, _ io.Writer) error {
				executions.Add(1)
				return nil
			}

			var stdout, stderr bytes.Buffer
			code := run([]string{"-jobs", test.rawJobs, "lint"}, &stdout, &stderr)
			if code != test.wantCode {
				t.Fatalf("run() exit code = %d, want %d; stderr = %q", code, test.wantCode, stderr.String())
			}
			if executions.Load() != 0 {
				t.Fatalf("checker execution count = %d, want validation to stop the lane first", executions.Load())
			}
			wantReceived := fmt.Sprintf("received %q", test.rawJobs)
			if !strings.Contains(stderr.String(), "invalid -jobs value") || !strings.Contains(stderr.String(), wantReceived) {
				t.Fatalf("validation diagnostic = %q, want -jobs and %q", stderr.String(), wantReceived)
			}
		})
	}
}

func TestParseConfigUsesDefaultJobsWhenOmitted(t *testing.T) {
	cfg, err := parseConfig([]string{"lint"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.jobs != defaultLintJobs {
		t.Fatalf("default jobs = %d, want %d", cfg.jobs, defaultLintJobs)
	}
}

func TestParseConfigPreservesTargetSelection(t *testing.T) {
	cfg, err := parseConfig([]string{"-make", "gmake", "-jobs", "3", "pkg-maint", "vet"}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.makeTool != "gmake" || cfg.jobs != 3 || !slices.Equal(cfg.targets, []string{"pkg-maint", "vet"}) {
		t.Fatalf("parseConfig() = %+v", cfg)
	}
}

func helperCommandArgs(argv []string) ([]string, bool) {
	for index, arg := range argv {
		if arg == "--" {
			return argv[index+1:], true
		}
	}
	return nil, false
}
