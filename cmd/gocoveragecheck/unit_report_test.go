package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var unitReportCoverPackages = []string{
	modulePath + "/pkg/config",
	modulePath + "/pkg/service",
	modulePath + "/pkg/wire",
}

// unitReportProfile has one below-floor package, one near-floor package, and
// one comfortably passing package so the ordered verdict block and the JSON
// coverage summary both have something to order.
func unitReportProfile() string {
	return strings.Join([]string{
		"mode: count",
		modulePath + "/pkg/config/config.go:1.1,2.1 1 1",
		modulePath + "/pkg/config/config.go:11.1,12.1 2 0",
		modulePath + "/pkg/config/load.go:21.1,22.1 1 0",
		modulePath + "/pkg/service/factory.go:1.1,2.1 9 1",
		modulePath + "/pkg/service/factory.go:31.1,32.1 1 0",
		modulePath + "/pkg/wire/wire.go:1.1,2.1 10 3",
		"",
	}, "\n")
}

func unitReportTimingEvents(t *testing.T, outcome string) string {
	t.Helper()
	events := []goTestTimingEvent{
		{Action: "run", Package: modulePath + "/pkg/config", Test: "TestFastUnit"},
		{Action: "output", Package: modulePath + "/pkg/config", Test: "TestFastUnit", Output: "=== RUN   TestFastUnit\n"},
		{Action: "pass", Package: modulePath + "/pkg/config", Test: "TestFastUnit", Elapsed: 0.25},
		{Action: "run", Package: modulePath + "/pkg/service", Test: "TestSlowUnit"},
	}
	if outcome == timingOutcomeFail {
		events = append(events,
			goTestTimingEvent{Action: "output", Package: modulePath + "/pkg/service", Test: "TestSlowUnit", Output: "    factory_test.go:41: want 3 workstations, got 2\n"},
			goTestTimingEvent{Action: "output", Package: modulePath + "/pkg/service", Test: "TestSlowUnit", Output: "--- FAIL: TestSlowUnit (12.50s)\n"},
		)
	}
	events = append(events,
		goTestTimingEvent{Action: outcome, Package: modulePath + "/pkg/service", Test: "TestSlowUnit", Elapsed: 12.5},
		goTestTimingEvent{Action: "output", Package: modulePath + "/pkg/config", Output: "ok  \t" + modulePath + "/pkg/config\t0.310s\n"},
		goTestTimingEvent{Action: "pass", Package: modulePath + "/pkg/config", Elapsed: 0.31},
		goTestTimingEvent{Action: outcome, Package: modulePath + "/pkg/service", Elapsed: 12.62},
		goTestTimingEvent{Action: "output", Package: modulePath + "/pkg/wire", Output: "ok  \t" + modulePath + "/pkg/wire\t0.040s\n"},
		goTestTimingEvent{Action: "pass", Package: modulePath + "/pkg/wire", Elapsed: 0.04},
	)

	var stream strings.Builder
	for _, event := range events {
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal timing event: %v", err)
		}
		stream.Write(encoded)
		stream.WriteString("\n")
	}
	return stream.String()
}

func unitReportManifest(t *testing.T) string {
	t.Helper()
	// Measured coverage is config 25.00, service 90.00, wire 100.00, so these
	// floors put all three inside the near-floor band with headroom ordering
	// (service 1.00, wire 1.00, config 1.50) deliberately unequal to import-path
	// ordering (config, service, wire).
	return writePackageMinimumManifestWithEntries(t, "unit", []manifestPackageSpec{
		{importPath: modulePath + "/pkg/config", minimum: "23.50"},
		{importPath: modulePath + "/pkg/service", minimum: "89.00"},
		{importPath: modulePath + "/pkg/wire", minimum: "99.00"},
	})
}

func unitReportConfig(manifestPath string, jsonPath string, timingPath string) config {
	return config{
		min:             0,
		suite:           "unit",
		coverpkg:        strings.Join(unitReportCoverPackages, ","),
		packages:        "./pkg/...",
		packageManifest: manifestPath,
		jsonOutput:      jsonPath,
		timingOutput:    timingPath,
	}
}

// captureUnitReportRun stubs the go test child with a fake that writes the
// shared profile and replays a recorded go test -json event stream, so the unit
// lane's JSON, timing, and console outputs are all observable.
func captureUnitReportRun(t *testing.T, cfg config, stream string, commandErr error) (string, string, error) {
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
	commandRunner = func(invocation commandInvocation) (string, string, error) {
		if invocation.name != "go" || len(invocation.args) == 0 || invocation.args[0] != "test" {
			return "", "", fmt.Errorf("unexpected command %q %v", invocation.name, invocation.args)
		}
		profilePath := helperCoverProfilePath(invocation.args[1:])
		if profilePath == "" {
			return "", "", errors.New("missing -coverprofile")
		}
		if err := writeFakeCoverageProfile(profilePath, unitReportProfile()); err != nil {
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

	err := execute(cfg)
	return stdout.String(), stderr.String(), err
}

func TestUnitCoverageLaneWritesCoverageAndTimingJSON(t *testing.T) {
	outputDir := t.TempDir()
	coveragePath := filepath.Join(outputDir, "coverage-summary.json")
	timingPath := filepath.Join(outputDir, "unit-timing-summary.json")

	stdout, _, err := captureUnitReportRun(
		t,
		unitReportConfig(unitReportManifest(t), coveragePath, timingPath),
		unitReportTimingEvents(t, timingOutcomePass),
		nil,
	)
	if err != nil {
		t.Fatalf("execute() error = %v\n%s", err, stdout)
	}

	var coverage struct {
		Packages []struct {
			Package         string   `json:"package"`
			CoveragePercent float64  `json:"coveragePercent"`
			PackageFloor    *float64 `json:"packageFloor"`
		} `json:"packages"`
	}
	readUnitReportJSON(t, coveragePath, &coverage)
	if len(coverage.Packages) != len(unitReportCoverPackages) {
		t.Fatalf("coverage summary packages = %d, want %d", len(coverage.Packages), len(unitReportCoverPackages))
	}
	floors := 0
	for _, entry := range coverage.Packages {
		if entry.PackageFloor != nil {
			floors++
		}
	}
	if floors != len(unitReportCoverPackages) {
		t.Fatalf("coverage summary carried %d package floors, want %d", floors, len(unitReportCoverPackages))
	}

	var timing struct {
		Tests []struct {
			Package string  `json:"package"`
			Test    string  `json:"test"`
			Seconds float64 `json:"seconds"`
			Outcome string  `json:"outcome"`
		} `json:"tests"`
	}
	readUnitReportJSON(t, timingPath, &timing)
	slowest := map[string]float64{}
	for _, entry := range timing.Tests {
		slowest[entry.Test] = entry.Seconds
	}
	if got := slowest["TestSlowUnit"]; got != 12.5 {
		t.Fatalf("TestSlowUnit elapsed = %v, want 12.5 (timing rows: %+v)", got, timing.Tests)
	}
	if got := slowest["TestFastUnit"]; got != 0.25 {
		t.Fatalf("TestFastUnit elapsed = %v, want 0.25 (timing rows: %+v)", got, timing.Tests)
	}
}

// TestUnitCoverageLaneCollapsesPerPackageLinesWhenTheArtifactIsWritten pins the
// console half of the unit report: once -json-output preserves the complete
// per-package measurement, stdout carries one ordered verdict block instead of
// a raw coverage line per measured package.
func TestUnitCoverageLaneCollapsesPerPackageLinesWhenTheArtifactIsWritten(t *testing.T) {
	outputDir := t.TempDir()
	coveragePath := filepath.Join(outputDir, "coverage-summary.json")

	stdout, _, err := captureUnitReportRun(
		t,
		unitReportConfig(unitReportManifest(t), coveragePath, filepath.Join(outputDir, "unit-timing-summary.json")),
		unitReportTimingEvents(t, timingOutcomePass),
		nil,
	)
	if err != nil {
		t.Fatalf("execute() error = %v\n%s", err, stdout)
	}

	if !strings.Contains(stdout, "Unit package coverage verdict:") {
		t.Fatalf("unit lane did not render its own verdict block:\n%s", stdout)
	}
	if strings.Contains(stdout, "Functional package coverage verdict:") {
		t.Fatalf("unit lane mislabelled its verdict block as functional:\n%s", stdout)
	}
	for _, importPath := range unitReportCoverPackages {
		if strings.Contains(stdout, importPath+"\tcoverage: ") {
			t.Fatalf("unit lane still printed a raw per-package coverage line for %s:\n%s", importPath, stdout)
		}
	}

	// All three packages sit inside the near-floor band with headroom service
	// 1.00, wire 1.00, config 1.50. Ordering is headroom ascending with an
	// import-path tiebreak, so the block must read service, wire, config —
	// deliberately not the import-path order config, service, wire.
	block := stdout[strings.Index(stdout, "Unit package coverage verdict:"):]
	var reported []string
	for _, line := range strings.Split(block, "\n") {
		if _, importPath, found := strings.Cut(strings.TrimSpace(line), "near floor: package="); found {
			reported = append(reported, strings.Fields(importPath)[0])
		}
	}
	want := []string{modulePath + "/pkg/service", modulePath + "/pkg/wire", modulePath + "/pkg/config"}
	if strings.Join(reported, ",") != strings.Join(want, ",") {
		t.Fatalf("near-floor order = %v, want headroom ascending %v:\n%s", reported, want, stdout)
	}

	// The collapsed log is only safe because the complete set survives in the
	// artifact the CI job uploads.
	var coverage struct {
		Packages []struct {
			Package string `json:"package"`
		} `json:"packages"`
	}
	readUnitReportJSON(t, coveragePath, &coverage)
	if len(coverage.Packages) != len(unitReportCoverPackages) {
		t.Fatalf("collapsed log lost packages: artifact carries %d, want %d", len(coverage.Packages), len(unitReportCoverPackages))
	}
}

// TestUnitCoverageLaneCreatesItsArtifactDirectory pins the CI shape: the three
// output paths point into an artifact directory that does not exist yet, and
// go test -coverprofile will not create it.
func TestUnitCoverageLaneCreatesItsArtifactDirectory(t *testing.T) {
	artifactRoot := filepath.Join(t.TempDir(), ".artifacts", "unit-coverage")

	cfg := unitReportConfig(
		unitReportManifest(t),
		filepath.Join(artifactRoot, "coverage-summary.json"),
		filepath.Join(artifactRoot, "unit-timing-summary.json"),
	)
	cfg.profile = filepath.Join(artifactRoot, "coverage.out")

	stdout, _, err := captureUnitReportRun(t, cfg, unitReportTimingEvents(t, timingOutcomePass), nil)
	if err != nil {
		t.Fatalf("execute() error = %v\n%s", err, stdout)
	}

	for _, name := range []string{"coverage.out", "coverage-summary.json", "unit-timing-summary.json"} {
		if _, statErr := os.Stat(filepath.Join(artifactRoot, name)); statErr != nil {
			t.Fatalf("unit lane did not publish %s into a fresh artifact directory: %v", name, statErr)
		}
	}
}

// TestUnitCoverageLaneNamesItsOwnLaneInItsCaptureNote pins the prose the unit
// report renders when a capture ends early. The note is written by code both
// suites share, and the rendered summary shows it to a reader who is looking
// at the unit job, so naming the functional suite there reads as evidence that
// the wrong lane was measured.
func TestUnitCoverageLaneNamesItsOwnLaneInItsCaptureNote(t *testing.T) {
	outputDir := t.TempDir()
	timingPath := filepath.Join(outputDir, "unit-timing-summary.json")

	stdout, _, err := captureUnitReportRun(
		t,
		unitReportConfig(
			unitReportManifest(t),
			filepath.Join(outputDir, "coverage-summary.json"),
			timingPath,
		),
		unitReportTimingEvents(t, timingOutcomePass),
		nil,
	)
	if err != nil {
		t.Fatalf("execute() error = %v\n%s", err, stdout)
	}

	var timing struct {
		Complete      bool   `json:"complete"`
		CaptureReason string `json:"captureReason"`
	}
	readUnitReportJSON(t, timingPath, &timing)
	if timing.Complete {
		t.Fatal("timing capture reported complete; this fixture must end early or it stops covering the capture note")
	}
	if !strings.HasPrefix(timing.CaptureReason, "unit ") {
		t.Fatalf("unit lane capture note = %q, want it to name the unit lane", timing.CaptureReason)
	}

	// The note is shared with the functional lane, whose wording this change
	// must leave byte-identical.
	if got := coverageLaneNoun(functionalCoverageSuite); got != "functional" {
		t.Fatalf("functional lane noun = %q, want the wording its reports already carry", got)
	}
}

// TestInterruptedTimingSnapshotEmitsAnEmptyTestArray pins the artifact an
// if: always() upload preserves when a lane is cut short. A snapshot has no
// per-test rows yet, and a nil slice marshals to JSON null, which a consumer
// reading a documented array field cannot tell apart from an unreadable
// document. Reporting an interrupted lane as unmeasurable is exactly the
// failure the always-on artifact exists to prevent.
func TestInterruptedTimingSnapshotEmitsAnEmptyTestArray(t *testing.T) {
	tracker := newFunctionalTimingTracker([]string{modulePath + "/pkg/config"}, time.Now())
	snapshot := tracker.snapshot(false, "lane budget expired", time.Now())

	if snapshot.Tests == nil {
		t.Fatal("snapshot Tests is nil; it marshals to JSON null and reads as an unavailable timing summary")
	}
	if len(snapshot.Tests) != 0 {
		t.Fatalf("snapshot Tests = %+v, want an empty array", snapshot.Tests)
	}

	data, err := renderFunctionalTimingSummaryJSON(snapshot)
	if err != nil {
		t.Fatalf("render timing snapshot: %v", err)
	}
	var decoded struct {
		Tests *[]functionalTestTimingJSON `json:"tests"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode timing snapshot: %v\n%s", err, data)
	}
	if decoded.Tests == nil {
		t.Fatalf("timing snapshot encoded tests as null:\n%s", data)
	}
}

func TestUnitCoverageLaneKeepsFailureDetailReadableWithTimingCapture(t *testing.T) {
	outputDir := t.TempDir()
	_, _, err := captureUnitReportRun(
		t,
		unitReportConfig(
			unitReportManifest(t),
			filepath.Join(outputDir, "coverage-summary.json"),
			filepath.Join(outputDir, "unit-timing-summary.json"),
		),
		unitReportTimingEvents(t, timingOutcomeFail),
		errors.New("exit status 1"),
	)
	if err == nil {
		t.Fatal("execute() error = nil, want the failing go test lane to fail the gate")
	}

	detail := err.Error()
	for _, want := range []string{
		"--- FAIL: TestSlowUnit (12.50s)",
		"factory_test.go:41: want 3 workstations, got 2",
	} {
		if !strings.Contains(detail, want) {
			t.Fatalf("failure detail dropped %q:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "\"Action\":\"output\"") {
		t.Fatalf("failure detail reported raw test2json events instead of the human go test stream:\n%s", detail)
	}
}

func readUnitReportJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode %s: %v\n%s", filepath.Base(path), err, data)
	}
}

// TestNonStreamingTimingObserverDoesNotWritePerEvent guards the unit lane's
// measured runtime. The snapshotter's observe path runs on the goroutine that
// drains the child go test pipe: when it published per event it serialized the
// whole run's snapshot and wrote it to disk once per package start and once per
// package terminal, and the child blocked on a full pipe for the duration.
//
// The functional lane hid this because it observes a few dozen packages. The
// unit lane observes hundreds, and the CI job went from ~5 minutes to over an
// hour without completing.
//
// The bound below is deliberately loose. Per-event publishing cost roughly
// 3.5ms per event locally, so this many events took about four seconds; leaving
// persistence to the ticker costs no I/O on this path at all and finishes in
// milliseconds. Anything near the old cost fails.
func TestNonStreamingTimingObserverDoesNotWritePerEvent(t *testing.T) {
	const packageCount = 528

	expected := make([]string, 0, packageCount)
	for index := range packageCount {
		expected = append(expected, fmt.Sprintf("%s/pkg/probe/pkg%04d", modulePath, index))
	}

	timingPath := filepath.Join(t.TempDir(), "unit-timing-summary.json")
	tracker := newFunctionalTimingTracker(expected, time.Now())
	// sink nil is the non-streaming unit lane: -timing-output without -stream.
	snapshotter := newFunctionalTimingSnapshotter(tracker, timingPath, nil, nil)

	started := time.Now()
	for _, packageName := range expected {
		snapshotter.observe(goTestTimingEvent{Action: "start", Package: packageName})
	}
	for _, packageName := range expected {
		snapshotter.observe(goTestTimingEvent{Action: timingOutcomePass, Package: packageName, Elapsed: 1.5})
	}
	elapsed := time.Since(started)

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("observing %d package events on the pipe-drain path took %s; it must not perform per-event I/O", packageCount*2, elapsed)
	}

	// The events still have to be recorded: this must be cheaper, not blinder.
	// An incomplete summary is the case the tracker exists for, because that is
	// when finish() falls back to tracker state to describe every package.
	if err := snapshotter.finish(functionalTimingSummaryJSON{Complete: false}, "lane interrupted"); err != nil {
		t.Fatalf("finish timing snapshot: %v", err)
	}

	data, err := os.ReadFile(timingPath)
	if err != nil {
		t.Fatalf("read timing summary: %v", err)
	}
	var document functionalTimingSummaryJSON
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode timing summary: %v", err)
	}
	if len(document.PackageStates) != packageCount {
		t.Fatalf("timing summary recorded %d package states, want %d", len(document.PackageStates), packageCount)
	}
	for _, state := range document.PackageStates {
		if state.State != functionalPackageStateCompleted {
			t.Fatalf("package %s state is %q, want %q", state.Package, state.State, functionalPackageStateCompleted)
		}
	}
}

// TestStreamingTimingObserverStillPublishesPerEvent pins the functional lane's
// behaviour in place: its progress lines are the live log a reader watches, so
// the per-event publish must survive for a lane that has a sink.
func TestStreamingTimingObserverStillPublishesPerEvent(t *testing.T) {
	packages := []string{modulePath + "/tests/functional/alpha", modulePath + "/tests/functional/beta"}
	timingPath := filepath.Join(t.TempDir(), "functional-timing-summary.json")
	tracker := newFunctionalTimingTracker(packages, time.Now())

	var sink strings.Builder
	snapshotter := newFunctionalTimingSnapshotter(tracker, timingPath, &sink, nil)
	snapshotter.observe(goTestTimingEvent{Action: "start", Package: packages[0]})
	snapshotter.stopAndWait()

	if !strings.Contains(sink.String(), "Functional timing snapshot:") {
		t.Fatalf("streaming lane emitted no progress line per event; got %q", sink.String())
	}
}

// TestIncompleteTimingCaptureNamesTheCauseItObserved covers the report accuracy
// this lane's first hosted run broke: the unit lane recorded terminal results
// for all 548 of its packages and still published "ended before every package
// reported a terminal result", because a single unparseable line marks the whole
// capture incomplete. A reader acts differently on a truncated run than on a
// lossy parse, so the message has to follow the counts rather than assume.
func TestIncompleteTimingCaptureNamesTheCauseItObserved(t *testing.T) {
	truncated := functionalTimingSummaryJSON{PackageCount: 12, ExpectedPackageCount: 548}
	if got := incompleteTimingCaptureCause(truncated); !strings.Contains(got, "ended before every package") {
		t.Fatalf("a lane missing packages must report truncation; got %q", got)
	}

	// Every expected package reported, so the capture cannot have ended early.
	lossy := functionalTimingSummaryJSON{PackageCount: 548, ExpectedPackageCount: 548}
	got := incompleteTimingCaptureCause(lossy)
	if strings.Contains(got, "ended before every package") {
		t.Fatalf("a lane that reported every package must not claim it ended early; got %q", got)
	}
	if !strings.Contains(got, "could not read every go test event") {
		t.Fatalf("a lossy parse must say so; got %q", got)
	}
}
