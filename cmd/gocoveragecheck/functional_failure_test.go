package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderFunctionalFailureDetailExcludesSuccessfulPackageOutput(t *testing.T) {
	successPackage := modulePath + "/tests/functional/quiet/success"
	failingPackage := modulePath + "/tests/functional/quiet/failure"
	stream := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: successPackage, Test: "TestGreen", Output: "=== RUN   TestGreen\nsuccessful test debug log\n--- PASS: TestGreen (0.01s)\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomePass, Package: successPackage, Test: "TestGreen", Elapsed: 0.01}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomePass, Package: successPackage, Elapsed: 0.02}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: failingPackage, Test: "TestBroken", Output: "=== RUN   TestBroken\nexpected 2 workstations, got 1\n--- FAIL: TestBroken (0.03s)\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomeFail, Package: failingPackage, Test: "TestBroken", Elapsed: 0.03}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomeFail, Package: failingPackage, Elapsed: 0.04}),
	}, "\n")

	got := renderFunctionalFailureDetail(stream)
	want := "functional test failure: package=" + failingPackage + " test=TestBroken reason=expected 2 workstations, got 1"
	if got != want {
		t.Fatalf("functional failure detail = %q, want %q", got, want)
	}
	if strings.Contains(got, successPackage) || strings.Contains(got, "successful test debug log") {
		t.Fatalf("functional failure detail retained successful-package output: %q", got)
	}
}

func TestFunctionalFailureDetailSortsMultipleFailedPackages(t *testing.T) {
	firstPackage := modulePath + "/tests/functional/quiet/zeta"
	secondPackage := modulePath + "/tests/functional/quiet/alpha"
	stream := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: firstPackage, Output: "panic: zeta failure\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomeFail, Package: firstPackage, Elapsed: 0.2}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: secondPackage, Output: "panic: alpha failure\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomeFail, Package: secondPackage, Elapsed: 0.1}),
	}, "\n")

	got := renderFunctionalFailureDetail(stream)
	want := strings.Join([]string{
		"functional test failure: package=" + secondPackage + " reason=panic: alpha failure",
		"functional test failure: package=" + firstPackage + " reason=panic: zeta failure",
	}, "\n")
	if got != want {
		t.Fatalf("multiple functional failure detail = %q, want sorted %q", got, want)
	}
}

func TestFunctionalRunSuppressesSuccessfulChildChatterAndKeepsArtifacts(t *testing.T) {
	packageNames := []string{
		modulePath + "/tests/functional/quiet/alpha",
		modulePath + "/tests/functional/quiet/beta",
	}
	stream := functionalQuietSuccessEventStream(packageNames)
	stdout, _, timingPath, err := captureFunctionalQuietRun(t, packageNames, stream, nil)
	if err != nil {
		t.Fatalf("functional quiet run error = %v\n%s", err, stdout)
	}

	for _, forbidden := range []string{
		"TestNoisy",
		"=== RUN",
		"--- PASS",
		"successful test debug log",
		"ok  ",
		"Functional package result:",
		"Functional timing snapshot:",
		`{"Action"`,
	} {
		if strings.Contains(stdout, forbidden) {
			t.Fatalf("functional quiet stdout contains %q:\n%s", forbidden, stdout)
		}
	}
	if strings.Count(stdout, functionalTimingReportHeader+"\n") != 1 {
		t.Fatalf("functional timing section count = %d, want one:\n%s", strings.Count(stdout, functionalTimingReportHeader+"\n"), stdout)
	}
	for _, packageName := range packageNames {
		row := "  package=" + packageName + " elapsed="
		if strings.Count(stdout, row) != 1 {
			t.Fatalf("functional timing row %q count = %d, want one:\n%s", row, strings.Count(stdout, row), stdout)
		}
	}
	if _, err := os.Stat(timingPath); err != nil {
		t.Fatalf("functional timing artifact was not written: %v", err)
	}
}

func TestFunctionalRunFailureNamesOnlyFailedPackage(t *testing.T) {
	packageNames := []string{
		modulePath + "/tests/functional/quiet/success",
		modulePath + "/tests/functional/quiet/failure",
	}
	stream := functionalQuietFailureEventStream(packageNames)
	stdout, _, _, err := captureFunctionalQuietRun(t, packageNames, stream, errors.New("exit status 1"))
	if err == nil {
		t.Fatal("functional quiet run unexpectedly succeeded")
	}

	detail := err.Error()
	if !strings.Contains(detail, "functional test failure: package="+packageNames[1]) ||
		!strings.Contains(detail, "test=TestBroken") ||
		!strings.Contains(detail, "reason=expected failure") {
		t.Fatalf("functional failure detail = %q, want failed package, test, and reason", detail)
	}
	if strings.Contains(detail, packageNames[0]) || strings.Contains(detail, "successful test debug log") {
		t.Fatalf("functional failure detail retained unrelated successful output: %q", detail)
	}
	if strings.Contains(stdout, "successful test debug log") || strings.Contains(stdout, "=== RUN") {
		t.Fatalf("functional failure stdout retained child chatter:\n%s", stdout)
	}
}

func captureFunctionalQuietRun(t *testing.T, packageNames []string, stream string, commandErr error) (string, string, string, error) {
	t.Helper()
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var stdout strings.Builder
	var stderr strings.Builder
	timingPath := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if invocation.name != "go" || len(invocation.args) == 0 || invocation.args[0] != "test" {
			return "", "", fmt.Errorf("unexpected invocation: %q %v", invocation.name, invocation.args)
		}
		profilePath := helperCoverProfilePath(invocation.args[1:])
		if profilePath == "" {
			return "", "", errors.New("missing -coverprofile")
		}
		if err := writeFakeCoverageProfile(profilePath, strings.Join([]string{
			"mode: count",
			modulePath + "/pkg/config/config.go:1.1,2.1 3 1",
			"",
		}, "\n")); err != nil {
			return "", "", err
		}
		if invocation.stdoutWriter != nil {
			if _, err := invocation.stdoutWriter.Write([]byte(stream)); err != nil {
				return "", "", err
			}
		}
		return stream, "", commandErr
	}
	stdoutWriter = &stdout
	stderrWriter = &stderr

	_, err := run(config{
		min:          0,
		suite:        functionalCoverageSuite,
		totalOnly:    true,
		stream:       true,
		coverpkg:     modulePath + "/pkg/config",
		packages:     strings.Join(packageNames, " "),
		profile:      filepath.Join(t.TempDir(), "coverage.out"),
		timingOutput: timingPath,
	})
	return stdout.String(), stderr.String(), timingPath, err
}

func functionalQuietSuccessEventStream(packageNames []string) string {
	var events []string
	for index, packageName := range packageNames {
		events = append(events,
			marshalGoTestEventOrPanic(goTestTimingEvent{Action: "run", Package: packageName, Test: "TestNoisy"}),
			marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: packageName, Test: "TestNoisy", Output: "=== RUN   TestNoisy\nsuccessful test debug log\n--- PASS: TestNoisy (0.01s)\n"}),
			marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomePass, Package: packageName, Test: "TestNoisy", Elapsed: 0.01}),
			marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: packageName, Output: "ok  \t" + packageName + "\t0.0" + fmt.Sprint(index+1) + "s\n"}),
			marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomePass, Package: packageName, Elapsed: float64(index+1) / 10}),
		)
	}
	return strings.Join(events, "\n") + "\n"
}

func functionalQuietFailureEventStream(packageNames []string) string {
	success := functionalQuietSuccessEventStream(packageNames[:1])
	failurePackage := packageNames[1]
	failure := strings.Join([]string{
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "run", Package: failurePackage, Test: "TestBroken"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: "output", Package: failurePackage, Test: "TestBroken", Output: "=== RUN   TestBroken\nexpected failure\n--- FAIL: TestBroken (0.02s)\n"}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomeFail, Package: failurePackage, Test: "TestBroken", Elapsed: 0.02}),
		marshalGoTestEventOrPanic(goTestTimingEvent{Action: timingOutcomeFail, Package: failurePackage, Elapsed: 0.03}),
	}, "\n")
	return success + failure + "\n"
}
