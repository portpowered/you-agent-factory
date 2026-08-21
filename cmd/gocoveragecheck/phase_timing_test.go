package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCoveragePhaseTimerEmitsStableLines(t *testing.T) {
	var output bytes.Buffer
	timer := newCoveragePhaseTimer(&output)
	start := time.Unix(100, 0)
	now := []time.Time{
		start,
		start.Add(1250 * time.Millisecond),
		start.Add(1250 * time.Millisecond),
		start.Add(3250 * time.Millisecond),
	}
	timer.now = func() time.Time {
		if len(now) == 0 {
			return start
		}
		value := now[0]
		now = now[1:]
		return value
	}

	if err := timer.measure(coveragePhaseList, func() error { return nil }); err != nil {
		t.Fatalf("list phase error = %v", err)
	}
	wantPlanError := errors.New("plan failed")
	if err := timer.measure(coveragePhasePlan, func() error { return wantPlanError }); !errors.Is(err, wantPlanError) {
		t.Fatalf("plan phase error = %v, want %v", err, wantPlanError)
	}
	timer.emit()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != len(coveragePhaseOrder) {
		t.Fatalf("timing lines = %d, want %d:\n%s", len(lines), len(coveragePhaseOrder), output.String())
	}
	wantLines := []string{
		"gocoveragecheck phase timing: phase=list duration=1.250s status=complete",
		"gocoveragecheck phase timing: phase=plan duration=2.000s status=error",
		"gocoveragecheck phase timing: phase=test duration=0.000s status=skipped",
		"gocoveragecheck phase timing: phase=canonicalize duration=0.000s status=skipped",
		"gocoveragecheck phase timing: phase=evaluate duration=0.000s status=skipped",
		"gocoveragecheck phase timing: phase=manifest duration=0.000s status=skipped",
	}
	if got := strings.Join(lines, "\n"); got != strings.Join(wantLines, "\n") {
		t.Fatalf("timing output =\n%s\nwant=\n%s", got, strings.Join(wantLines, "\n"))
	}

	timer.emit()
	if got := strings.Count(output.String(), "gocoveragecheck phase timing:"); got != len(coveragePhaseOrder) {
		t.Fatalf("emitting twice produced %d lines, want %d", got, len(coveragePhaseOrder))
	}
}

func TestExecuteEmitsCoveragePhasesWhenTestFails(t *testing.T) {
	originalCommandRunner := commandRunner
	originalStdout := stdoutWriter
	originalStderr := stderrWriter
	t.Cleanup(func() {
		commandRunner = originalCommandRunner
		stdoutWriter = originalStdout
		stderrWriter = originalStderr
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	commandRunner = fakeGoCoverageCommandTestFailsWithoutDetail
	stdoutWriter = &stdout
	stderrWriter = &stderr

	err := execute(config{
		suite:    unitCoverageSuite,
		coverpkg: modulePath + "/pkg/config",
		packages: "./pkg/config",
	})
	if err == nil {
		t.Fatal("execute() unexpectedly succeeded")
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != len(coveragePhaseOrder) {
		t.Fatalf("timing lines = %d, want %d:\n%s", len(lines), len(coveragePhaseOrder), stdout.String())
	}
	for _, name := range coveragePhaseOrder {
		marker := "phase=" + string(name) + " "
		count := 0
		for _, line := range lines {
			if strings.Contains(line, marker) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("phase %q occurred %d times in timing output:\n%s", name, count, stdout.String())
		}
	}
	testLine := ""
	for _, line := range lines {
		if strings.Contains(line, "phase=test ") {
			testLine = line
			break
		}
	}
	if !strings.Contains(testLine, "phase=test duration=") || !strings.Contains(testLine, "status=error") {
		t.Fatalf("test error phase was not emitted as an error:\n%s", stdout.String())
	}
}
