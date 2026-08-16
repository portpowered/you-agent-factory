package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFunctionalTimingTrackerSnapshotNamesActiveAndUnobservedPackages(t *testing.T) {
	started := time.Unix(100, 0)
	tracker := newFunctionalTimingTracker([]string{"pkg/alpha", "pkg/beta", "pkg/gamma"}, started)
	tracker.now = func() time.Time { return started }

	if !tracker.observe(goTestTimingEvent{Action: "start", Package: "pkg/alpha"}) {
		t.Fatal("start event was not observed")
	}
	tracker.now = func() time.Time { return started.Add(4 * time.Second) }
	if !tracker.observe(goTestTimingEvent{Action: timingOutcomePass, Package: "pkg/beta", Elapsed: 1.25}) {
		t.Fatal("terminal event was not observed")
	}

	summary := tracker.snapshot(false, "tier budget expired", started.Add(4*time.Second))
	if summary.Complete {
		t.Fatal("snapshot marked complete after an interrupted run")
	}
	if len(summary.PackageStates) != 3 {
		t.Fatalf("package states = %+v, want all expected packages", summary.PackageStates)
	}
	assertPackageState(t, summary.PackageStates[0], "pkg/alpha", functionalPackageStateInFlight, 4)
	assertPackageState(t, summary.PackageStates[1], "pkg/beta", functionalPackageStateCompleted, 1.25)
	assertPackageState(t, summary.PackageStates[2], "pkg/gamma", functionalPackageStateUnobserved, 0)
}

func TestFunctionalTimingSnapshotWritesMachineReadablePartialArtifact(t *testing.T) {
	started := time.Unix(200, 0)
	tracker := newFunctionalTimingTracker([]string{"pkg/hung"}, started)
	tracker.now = func() time.Time { return started.Add(3 * time.Second) }
	path := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	snapshotter := newFunctionalTimingSnapshotter(tracker, path, nil, nil)
	snapshotter.publish(false, "tier budget expired", false)
	snapshotter.stopAndWait()
	if err := snapshotter.writeError(); err != nil {
		t.Fatalf("snapshot write error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read timing snapshot: %v", err)
	}
	var summary functionalTimingSummaryJSON
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("decode timing snapshot: %v\n%s", err, data)
	}
	if summary.Complete || summary.CaptureReason != "tier budget expired" {
		t.Fatalf("partial timing summary = %+v, want incomplete timeout state", summary)
	}
	if len(summary.PackageStates) != 1 || summary.PackageStates[0].State != functionalPackageStateUnobserved {
		t.Fatalf("partial package states = %+v, want unobserved package", summary.PackageStates)
	}
}

func TestWritePartialCoverageSnapshotSkipsUntrustworthyProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "coverage-summary.json")
	err := writePartialCoverageSnapshot(
		path,
		filepath.Join(t.TempDir(), "missing-coverage.out"),
		".",
		[]string{modulePath + "/pkg/config"},
		"tier budget expired",
	)
	if err != nil {
		t.Fatalf("writePartialCoverageSnapshot() error = %v, want no error for unavailable profile", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected partial coverage artifact, stat err=%v", err)
	}
}

func TestWriteFunctionalTimingSummaryAtomicFailureNamesOutputDirectory(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(parent, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}
	err := writeFunctionalTimingSummaryJSON(filepath.Join(parent, "timing.json"), functionalTimingSummaryJSON{Version: functionalTimingSummaryVersion})
	if err == nil || !strings.Contains(err.Error(), "create diagnostic output directory") {
		t.Fatalf("writeFunctionalTimingSummaryJSON() error = %v, want actionable directory failure", err)
	}
}

func assertPackageState(t *testing.T, got functionalPackageStateJSON, packageName, state string, seconds float64) {
	t.Helper()
	if got.Package != packageName || got.State != state || got.Seconds != seconds {
		t.Fatalf("package state = %+v, want package=%s state=%s seconds=%v", got, packageName, state, seconds)
	}
}
