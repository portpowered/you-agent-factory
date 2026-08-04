package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFunctionalTimingSummarySuccessfulMultiPackageCapture(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/tests/functional/alpha"
	pkgB := modulePath + "/tests/functional/beta"
	pkgC := modulePath + "/tests/functional/gamma"

	jsonOutput := strings.Join([]string{
		goTestEventLine(t, goTestTimingEvent{Action: "run", Package: pkgB}),
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgB, Test: "TestOne", Elapsed: 0.01}),
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgB, Elapsed: 1.5}),
		goTestEventLine(t, goTestTimingEvent{Action: "fail", Package: pkgA, Elapsed: 0.75}),
		goTestEventLine(t, goTestTimingEvent{Action: "skip", Package: pkgC, Elapsed: 0}),
		"",
	}, "\n")

	summary := buildFunctionalTimingSummary(jsonOutput, []string{pkgA, pkgB, pkgC}, 2.25)

	if !summary.Complete {
		t.Fatalf("Complete = false, want true, summary = %+v", summary)
	}
	if summary.Version != functionalTimingSummaryVersion {
		t.Fatalf("Version = %d, want %d", summary.Version, functionalTimingSummaryVersion)
	}
	if summary.WallSeconds != 2.25 {
		t.Fatalf("WallSeconds = %v, want 2.25", summary.WallSeconds)
	}
	if summary.PackageCount != 3 {
		t.Fatalf("PackageCount = %d, want 3", summary.PackageCount)
	}
	wantSum := roundTimingSeconds(1.5 + 0.75 + 0)
	if summary.PackageElapsedSecondsSum != wantSum {
		t.Fatalf("PackageElapsedSecondsSum = %v, want %v", summary.PackageElapsedSecondsSum, wantSum)
	}

	// Deterministic ordering: alphabetical by package path regardless of
	// arrival order in the event stream.
	wantOrder := []string{pkgA, pkgB, pkgC}
	if len(summary.Packages) != len(wantOrder) {
		t.Fatalf("Packages len = %d, want %d", len(summary.Packages), len(wantOrder))
	}
	for i, wantPackage := range wantOrder {
		if summary.Packages[i].Package != wantPackage {
			t.Fatalf("Packages[%d] = %q, want %q", i, summary.Packages[i].Package, wantPackage)
		}
	}

	if summary.Packages[0].Outcome != timingOutcomeFail || summary.Packages[0].Seconds != 0.75 {
		t.Fatalf("Packages[0] = %+v, want fail/0.75", summary.Packages[0])
	}
	if summary.Packages[1].Outcome != timingOutcomePass || summary.Packages[1].Seconds != 1.5 {
		t.Fatalf("Packages[1] = %+v, want pass/1.5", summary.Packages[1])
	}
	if summary.Packages[2].Outcome != timingOutcomeSkip || summary.Packages[2].Seconds != 0 {
		t.Fatalf("Packages[2] = %+v, want skip/0", summary.Packages[2])
	}
}

func TestBuildFunctionalTimingSummaryConcurrentDurationSemantics(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/tests/functional/alpha"
	pkgB := modulePath + "/tests/functional/beta"

	jsonOutput := strings.Join([]string{
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgA, Elapsed: 3.0}),
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgB, Elapsed: 4.0}),
		"",
	}, "\n")

	// Packages ran concurrently: their elapsed sum (7.0s) exceeds the wall
	// clock (5.0s) measured around the whole invocation.
	summary := buildFunctionalTimingSummary(jsonOutput, []string{pkgA, pkgB}, 5.0)

	if !summary.Complete {
		t.Fatalf("Complete = false, want true")
	}
	if summary.WallSeconds != 5.0 {
		t.Fatalf("WallSeconds = %v, want 5.0", summary.WallSeconds)
	}
	if summary.PackageElapsedSecondsSum != 7.0 {
		t.Fatalf("PackageElapsedSecondsSum = %v, want 7.0", summary.PackageElapsedSecondsSum)
	}
	if summary.PackageElapsedSecondsSum <= summary.WallSeconds {
		t.Fatalf("expected package elapsed sum (%v) to exceed wall seconds (%v) under concurrency", summary.PackageElapsedSecondsSum, summary.WallSeconds)
	}
}

func TestBuildFunctionalTimingSummaryMissingPackageMarksIncomplete(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/tests/functional/alpha"
	pkgB := modulePath + "/tests/functional/beta"

	jsonOutput := goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgA, Elapsed: 1.0})

	summary := buildFunctionalTimingSummary(jsonOutput, []string{pkgA, pkgB}, 1.0)

	if summary.Complete {
		t.Fatalf("Complete = true, want false when a discovered package never reports a terminal event")
	}
	if summary.PackageCount != 1 {
		t.Fatalf("PackageCount = %d, want 1 (only the captured package)", summary.PackageCount)
	}
	if len(summary.Packages) != 1 || summary.Packages[0].Package != pkgA {
		t.Fatalf("Packages = %+v, want only %q", summary.Packages, pkgA)
	}
}

func TestBuildFunctionalTimingSummaryMalformedLineMarksIncomplete(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/tests/functional/alpha"

	jsonOutput := strings.Join([]string{
		"{not valid json",
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgA, Elapsed: 1.0}),
		"",
	}, "\n")

	summary := buildFunctionalTimingSummary(jsonOutput, []string{pkgA}, 1.0)

	if summary.Complete {
		t.Fatal("Complete = true, want false for a malformed event line")
	}
	if len(summary.Packages) != 1 || summary.Packages[0].Package != pkgA {
		t.Fatalf("Packages = %+v, want partial capture of %q preserved", summary.Packages, pkgA)
	}
}

func TestBuildFunctionalTimingSummaryDuplicateTerminalEventMarksIncomplete(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/tests/functional/alpha"

	jsonOutput := strings.Join([]string{
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgA, Elapsed: 1.0}),
		goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgA, Elapsed: 2.0}),
		"",
	}, "\n")

	summary := buildFunctionalTimingSummary(jsonOutput, []string{pkgA}, 2.0)

	if summary.Complete {
		t.Fatal("Complete = true, want false for a duplicate package-terminal event")
	}
	if len(summary.Packages) != 1 || summary.Packages[0].Seconds != 1.0 {
		t.Fatalf("Packages = %+v, want first terminal event (1.0s) retained", summary.Packages)
	}
}

func TestBuildFunctionalTimingSummaryNegativeElapsedMarksIncomplete(t *testing.T) {
	t.Parallel()

	pkgA := modulePath + "/tests/functional/alpha"

	jsonOutput := goTestEventLine(t, goTestTimingEvent{Action: "pass", Package: pkgA, Elapsed: -1.0})

	summary := buildFunctionalTimingSummary(jsonOutput, []string{pkgA}, 1.0)

	if summary.Complete {
		t.Fatal("Complete = true, want false for negative elapsed")
	}
	if len(summary.Packages) != 0 {
		t.Fatalf("Packages = %+v, want none captured from a negative-elapsed event", summary.Packages)
	}
}

func TestBuildFunctionalTimingSummaryEmptyExpectedPackagesIsComplete(t *testing.T) {
	t.Parallel()

	summary := buildFunctionalTimingSummary("", nil, 0)
	if !summary.Complete {
		t.Fatal("Complete = false, want true when no packages are expected")
	}
	if summary.PackageCount != 0 || len(summary.Packages) != 0 {
		t.Fatalf("Packages = %+v, want empty", summary.Packages)
	}
}

func TestRunWritesFunctionalTimingSummaryOnSuccess(t *testing.T) {
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
	commandRunner = fakeGoCoverageCommandWithTiming
	stdoutWriter = &stdout
	stderrWriter = &stderr

	timingPath := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	_, err := run(config{
		min: 0,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages:     modulePath + "/pkg/config",
		timingOutput: timingPath,
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}

	data, readErr := os.ReadFile(timingPath)
	if readErr != nil {
		t.Fatalf("read timing summary json: %v", readErr)
	}
	var summary functionalTimingSummaryJSON
	if decodeErr := json.Unmarshal(data, &summary); decodeErr != nil {
		t.Fatalf("decode timing summary json: %v\n%s", decodeErr, data)
	}
	if !summary.Complete {
		t.Fatalf("Complete = false, want true, summary = %+v", summary)
	}
	if summary.PackageCount != 1 {
		t.Fatalf("PackageCount = %d, want 1", summary.PackageCount)
	}
	if summary.Packages[0].Package != modulePath+"/pkg/config" {
		t.Fatalf("Packages[0].Package = %q, want %q", summary.Packages[0].Package, modulePath+"/pkg/config")
	}
	if summary.Packages[0].Outcome != timingOutcomePass {
		t.Fatalf("Packages[0].Outcome = %q, want pass", summary.Packages[0].Outcome)
	}
}

func TestRunOmitsFunctionalTimingSummaryWhenOptionAbsent(t *testing.T) {
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

	timingPath := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	_, err := run(config{
		min: 0,
		coverpkg: strings.Join([]string{
			modulePath + "/pkg/config",
			modulePath + "/pkg/service",
		}, ","),
		packages: "./pkg/config",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if _, statErr := os.Stat(timingPath); !os.IsNotExist(statErr) {
		t.Fatalf("unexpected timing file at %s when -timing-output absent (stat err=%v)", timingPath, statErr)
	}
}

func TestRunPreservesTimingForFailedPackageWhileKeepingLaneFailed(t *testing.T) {
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
	commandRunner = fakeGoCoverageCommandWithFailingTiming
	stdoutWriter = &stdout
	stderrWriter = &stderr

	timingPath := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	_, err := run(config{
		min:          0,
		coverpkg:     modulePath + "/pkg/config",
		packages:     modulePath + "/pkg/config",
		profile:      filepath.Join(t.TempDir(), "coverage.out"),
		timingOutput: timingPath,
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "run go test coverage lane") {
		t.Fatalf("run() error = %q, want go test coverage lane failure", err.Error())
	}

	// The lane fails because the package test failed, but the failed
	// package still reported a real terminal event, so its timing is a
	// trustworthy, complete diagnostic rather than a missing one.
	data, readErr := os.ReadFile(timingPath)
	if readErr != nil {
		t.Fatalf("read timing summary json after failed lane: %v", readErr)
	}
	var summary functionalTimingSummaryJSON
	if decodeErr := json.Unmarshal(data, &summary); decodeErr != nil {
		t.Fatalf("decode timing summary json: %v\n%s", decodeErr, data)
	}
	if !summary.Complete {
		t.Fatalf("Complete = false, want true for a fully-reported failed package, summary = %+v", summary)
	}
	if len(summary.Packages) != 1 || summary.Packages[0].Outcome != timingOutcomeFail || summary.Packages[0].Seconds != 0.4 {
		t.Fatalf("Packages = %+v, want one failed package retained with elapsed 0.4", summary.Packages)
	}
}

func TestRunWritesIncompleteFunctionalTimingSummaryWhenEventStreamTruncated(t *testing.T) {
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
	commandRunner = fakeGoCoverageCommandWithTruncatedTiming
	stdoutWriter = &stdout
	stderrWriter = &stderr

	timingPath := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	_, err := run(config{
		min:          0,
		coverpkg:     modulePath + "/pkg/config",
		packages:     modulePath + "/pkg/config",
		profile:      filepath.Join(t.TempDir(), "coverage.out"),
		timingOutput: timingPath,
	})
	if err == nil {
		t.Fatal("run() unexpectedly succeeded")
	}

	data, readErr := os.ReadFile(timingPath)
	if readErr != nil {
		t.Fatalf("read timing summary json after truncated lane: %v", readErr)
	}
	var summary functionalTimingSummaryJSON
	if decodeErr := json.Unmarshal(data, &summary); decodeErr != nil {
		t.Fatalf("decode timing summary json: %v\n%s", decodeErr, data)
	}
	if summary.Complete {
		t.Fatal("Complete = true, want false when no package ever reported a terminal event")
	}
	if len(summary.Packages) != 0 {
		t.Fatalf("Packages = %+v, want none captured from a build failure before any terminal event", summary.Packages)
	}
}

func TestRenderFunctionalTimingSummaryJSONIsDeterministic(t *testing.T) {
	t.Parallel()

	summary := functionalTimingSummaryJSON{
		Version:                  functionalTimingSummaryVersion,
		Complete:                 true,
		WallSeconds:              1.234,
		PackageElapsedSecondsSum: 2.5,
		PackageCount:             1,
		Packages: []functionalPackageTimingJSON{
			{Package: modulePath + "/tests/functional/alpha", Seconds: 2.5, Outcome: timingOutcomePass},
		},
	}

	first, err := renderFunctionalTimingSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderFunctionalTimingSummaryJSON() error = %v", err)
	}
	second, err := renderFunctionalTimingSummaryJSON(summary)
	if err != nil {
		t.Fatalf("renderFunctionalTimingSummaryJSON() second error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("timing summary json was not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}
}

func goTestEventLine(t *testing.T, event goTestTimingEvent) string {
	t.Helper()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal go test timing event: %v", err)
	}
	return string(data)
}

func marshalGoTestEventOrPanic(event goTestTimingEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// fakeGoCoverageCommandWithTiming simulates a passing `go test -json
// -coverprofile=...` invocation for a single package, used to exercise the
// gocoveragecheck timing-capture wiring end to end.
func fakeGoCoverageCommandWithTiming(invocation commandInvocation) (string, string, error) {
	if invocation.name != "go" || len(invocation.args) == 0 || invocation.args[0] != "test" {
		return "", "", fmt.Errorf("unexpected invocation: %+v", invocation)
	}
	profilePath := helperCoverProfilePath(invocation.args)
	if profilePath == "" {
		return "", "", fmt.Errorf("missing -coverprofile")
	}
	if err := writeFakeCoverageProfile(profilePath, strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
		modulePath + "/pkg/service/factory.go:1.1,2.1 5 2",
		"",
	}, "\n")); err != nil {
		return "", "", err
	}
	stdout := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "run", Package: modulePath + "/pkg/config"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "pass", Package: modulePath + "/pkg/config", Test: "TestConfig", Elapsed: 0.02}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "pass", Package: modulePath + "/pkg/config", Elapsed: 0.5}),
		"",
	}, "\n")
	return stdout, "", nil
}

// fakeGoCoverageCommandWithTruncatedTiming simulates a build failure that
// exits non-zero before go test emits any JSON events at all (e.g. a compile
// error), leaving the -json stdout stream empty of terminal package events.
func fakeGoCoverageCommandWithTruncatedTiming(invocation commandInvocation) (string, string, error) {
	if invocation.name != "go" || len(invocation.args) == 0 || invocation.args[0] != "test" {
		return "", "", fmt.Errorf("unexpected invocation: %+v", invocation)
	}
	return "", "# github.com/portpowered/infinite-you/pkg/config\npkg/config/config.go:1:1: syntax error", fmt.Errorf("exit status 2")
}

// fakeGoCoverageCommandWithFailingTiming simulates a failing `go test -json`
// invocation that reports a package failure and exits non-zero, verifying
// timing capture still records the failed package's terminal event.
func fakeGoCoverageCommandWithFailingTiming(invocation commandInvocation) (string, string, error) {
	if invocation.name != "go" || len(invocation.args) == 0 || invocation.args[0] != "test" {
		return "", "", fmt.Errorf("unexpected invocation: %+v", invocation)
	}
	stdout := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "run", Package: modulePath + "/pkg/config"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: modulePath + "/pkg/config", Test: "TestConfig", Elapsed: 0.1}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "fail", Package: modulePath + "/pkg/config", Elapsed: 0.4}),
		"",
	}, "\n")
	return stdout, "", fmt.Errorf("exit status 1")
}
